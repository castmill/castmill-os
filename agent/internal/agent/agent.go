// Package agent is the castmill-agent daemon (spec 07, as revised by
// ADR-0008): the native side of the Castmill device app's Machine interface,
// OS updates, player supervision and the state castmill-health reads.
//
// Pairing, content, remote commands and the live connection to Castmill are
// handled by Castmill's own device app, which runs in the kiosk shell and
// reaches the agent through the castmillOS bridge
// (docs/castmill-integration.md).
package agent

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/castmill/castmill-os/agent/internal/api"
	"github.com/castmill/castmill-os/agent/internal/config"
	"github.com/castmill/castmill-os/agent/internal/display"
	"github.com/castmill/castmill-os/agent/internal/maintenance"
	"github.com/castmill/castmill-os/agent/internal/playerlink"
	"github.com/castmill/castmill-os/agent/internal/rauc"
	"github.com/castmill/castmill-os/agent/internal/state"
	"github.com/castmill/castmill-os/agent/internal/system"
	"github.com/castmill/castmill-os/agent/internal/telemetry"
	"github.com/castmill/castmill-os/agent/internal/updater"
)

// Version is set at build time (-ldflags "-X ...agent.Version=...").
var Version = "0.1.0-dev"

// Agent is the running daemon.
type Agent struct {
	Cfg     *config.Agent
	Log     *slog.Logger
	RAUC    rauc.Client    // nil disables updates (e.g. running on a workstation)
	Systemd system.Manager // nil disables unit control and reboots
	SysRoot string         // "/" on a device

	machineID string
	prov      *config.Provisioning
	backend   string
	http      *http.Client
	player    *playerlink.Server
	upd       *updater.Updater
	sampler   *telemetry.Sampler

	mu         sync.Mutex
	devcfg     api.DeviceConfig
	rt         state.Runtime
	osVersion  string
	updateKick chan struct{}
	recovery   map[string]*recoveryState

	display      *display.Controller
	displayKick  chan struct{}
	schedules    map[string]display.Timers
	displayPower map[string]*state.DisplayState

	gpu map[string]state.GPUState // GPU reports per output
}

type recoveryState struct {
	reloadedAt   time.Time
	restartedAt  time.Time
	missingSince time.Time
}

// DesiredPath is where "castmill-agent update" (and later the Castmill OS
// update integration) puts the version the device should run.
func DesiredPath(cfg *config.Agent) string { return filepath.Join(cfg.StateDir(), "desired.json") }

// Run blocks until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) error {
	if a.SysRoot == "" {
		a.SysRoot = "/"
	}
	a.updateKick = make(chan struct{}, 1)
	a.displayKick = make(chan struct{}, 1)
	a.recovery = map[string]*recoveryState{}
	a.display = &display.Controller{SysRoot: a.SysRoot, DevRoot: filepath.Join(a.SysRoot, "dev")}
	a.loadDisplaySchedules()
	for _, d := range []string{a.Cfg.Paths.RunDir, a.Cfg.StateDir(), a.Cfg.AgentConfigDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	var err error
	if a.machineID, err = loadOrCreateMachineID(filepath.Join(a.Cfg.StateDir(), "machine-id")); err != nil {
		return fmt.Errorf("machine id: %w", err)
	}
	if a.prov, err = config.LoadProvisioning(a.Cfg.ProvisioningPath()); err != nil {
		a.Log.Warn("provisioning file unreadable", "err", err)
		a.prov = &config.Provisioning{}
	}
	_ = config.ReadJSON(a.Cfg.DeviceConfigPath(), &a.devcfg)
	a.osVersion = osRelease(a.SysRoot)["VERSION_ID"]
	if err := a.setupBackend(); err != nil {
		return err
	}
	a.mu.Lock()
	a.rt = state.Runtime{AgentVersion: Version, DeviceName: a.deviceName(), BackendURL: a.backend,
		Outputs: map[string]state.OutputState{}}
	a.mu.Unlock()

	if a.player, err = playerlink.Listen(a.Cfg.PlayerSocketPath(), a.call, a.Log.With("c", "player")); err != nil {
		return fmt.Errorf("player socket: %w", err)
	}
	go a.player.Serve(ctx)
	a.writePlayerConfig()
	a.sampler = &telemetry.Sampler{Root: a.SysRoot, DataPath: a.Cfg.Paths.DataRoot}

	if a.RAUC != nil {
		var rebooter updater.Rebooter
		if a.Systemd != nil {
			rebooter = a.Systemd
		}
		a.upd = &updater.Updater{
			RAUC: versionedRAUC{a.RAUC, a.osVersion}, Rebooter: rebooter, Log: a.Log.With("c", "update"),
			StatePath:  filepath.Join(a.Cfg.StateDir(), "update.json"),
			HealthPath: a.Cfg.HealthReportPath(),
			Policy:     a.policy,
		}
		if err := a.upd.Resume(ctx); err != nil {
			a.Log.Error("update state", "err", err)
		}
		go a.updateLoop(ctx)
	}

	go a.frameWatchdog(ctx)
	go a.probeLoop(ctx)
	go a.displayLoop(ctx)
	_ = system.Notify("READY=1\nSTATUS=running")
	a.publishLoop(ctx)
	return nil
}

// setupBackend picks the Castmill backend (castmill.conf may override it on
// development images only) and the HTTP client for the reachability probe.
func (a *Agent) setupBackend() error {
	a.backend = strings.TrimRight(a.Cfg.Backend.URL, "/")
	caPEM := ""
	caFile := a.Cfg.Backend.CAFile
	if a.Cfg.Backend.AllowOverride && a.prov.BackendURL != "" {
		a.backend = strings.TrimRight(a.prov.BackendURL, "/")
		caPEM, caFile = a.prov.BackendCA, ""
		a.Log.Warn("using the backend from castmill.conf (development image)", "url", a.backend)
	}
	var roots *x509.CertPool
	if caFile != "" || caPEM != "" {
		roots = x509.NewCertPool()
		if caFile != "" {
			b, err := os.ReadFile(caFile)
			if err != nil {
				return err
			}
			roots.AppendCertsFromPEM(b)
		}
		if caPEM != "" && !roots.AppendCertsFromPEM([]byte(caPEM)) {
			return errors.New("backend CA override: no certificates")
		}
	}
	a.http = &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{
		Proxy:           http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots},
	}}
	return nil
}

// PlayerURL is the Castmill device app served by the backend.
func (a *Agent) PlayerURL() string { return a.backend + "/" }

func (a *Agent) deviceName() string {
	if len(a.machineID) >= 8 {
		return "castmill-os-" + a.machineID[:8]
	}
	return "castmill-os"
}

func loadOrCreateMachineID(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(b)); len(id) == 32 {
			return id, nil
		}
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	id := hex.EncodeToString(buf)
	return id, config.WriteFileAtomic(path, []byte(id+"\n"), 0o644)
}

// ---- castmillOS bridge (Machine calls from the Castmill device app) -----------

// DeviceInfo is the OS part of Castmill's DeviceInfo; the kiosk shell adds
// the Electron/Chromium versions and the device app the user agent.
type DeviceInfo struct {
	AppType    string `json:"appType"`
	AppVersion string `json:"appVersion"`
	OS         string `json:"os"`
	Hardware   string `json:"hardware"`
}

func (a *Agent) call(ctx context.Context, output, method string, args json.RawMessage) (any, error) {
	switch method {
	case "set_timers":
		return nil, a.setTimers(output, args)
	case "gpu_report":
		return nil, a.gpuReport(output, args)
	case "machine_id":
		// One Castmill device per output: independent content per screen (MED-005).
		return a.machineID + "-" + output, nil
	case "device_info":
		return a.deviceInfo(), nil
	case "hardware_info":
		return telemetry.ReadHardware(a.SysRoot), nil
	case "telemetry":
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.sampler.Sample(), nil
	case "timezone":
		return a.timezone(), nil
	case "reboot":
		if a.Systemd == nil {
			return nil, errors.New("reboot not available")
		}
		a.Log.Warn("reboot requested by the Castmill device app", "output", output)
		go func() {
			time.Sleep(time.Second) // let the reply reach the player first
			if err := a.Systemd.Reboot(context.Background()); err != nil {
				a.Log.Error("reboot", "err", err)
			}
		}()
		return nil, nil
	}
	return nil, fmt.Errorf("unknown method %q", method)
}

func (a *Agent) deviceInfo() DeviceInfo {
	osName := "Castmill OS"
	if a.osVersion != "" {
		osName += " " + a.osVersion
	}
	hw := a.Cfg.Device.Machine
	var h struct{ Model string }
	if config.ReadJSON(filepath.Join(a.Cfg.ConfigDir(), "hardware.json"), &h) == nil && h.Model != "" {
		hw = h.Model
	}
	return DeviceInfo{AppType: "Castmill OS", AppVersion: a.osVersion, OS: osName, Hardware: hw}
}

// timezone returns the IANA name the localtime symlink points at.
func (a *Agent) timezone() string {
	target, err := os.Readlink(a.Cfg.LocaltimePath())
	if err != nil {
		return "UTC"
	}
	if i := strings.Index(target, "zoneinfo/"); i >= 0 {
		return target[i+len("zoneinfo/"):]
	}
	return "UTC"
}

// ---- backend reachability ----------------------------------------------------

// probeLoop checks that the Castmill backend answers (castmill-health needs
// it during a trial boot) and takes the time from its Date header when NTP
// is blocked (spec 05 "Time").
func (a *Agent) probeLoop(ctx context.Context) {
	for {
		err := a.probe(ctx)
		a.mu.Lock()
		if err == nil {
			a.rt.BackendOK, a.rt.BackendError = time.Now().UTC(), ""
		} else {
			a.rt.BackendError = err.Error()
		}
		a.mu.Unlock()
		wait := time.Minute
		if err != nil {
			wait = 20 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func (a *Agent) probe(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, a.PlayerURL(), nil)
	if err != nil {
		return err
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if d, err := http.ParseTime(resp.Header.Get("Date")); err == nil && !system.TimeTrusted(a.Cfg.Paths.RunDir) {
		if err := system.SetClockFromBackend(a.Cfg.Paths.RunDir, d); err != nil {
			a.Log.Debug("clock from backend", "err", err)
		}
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("backend answered HTTP %d", resp.StatusCode)
	}
	return nil
}

// ---- updates -----------------------------------------------------------------

func (a *Agent) updateLoop(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		var d api.Desired
		if err := config.ReadJSON(DesiredPath(a.Cfg), &d); err == nil {
			a.upd.SetDesired(&d)
			if d.ActivatePolicy == api.ActivateNow {
				_ = a.upd.ForceActivate()
			}
		}
		if err := a.upd.Step(ctx); err != nil {
			a.Log.Warn("update step", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-a.updateKick:
		}
	}
}

func (a *Agent) policy() maintenance.Policy {
	a.mu.Lock()
	c := a.devcfg
	a.mu.Unlock()
	loc := time.Local
	if l, err := time.LoadLocation(a.timezone()); err == nil {
		loc = l
	}
	return maintenance.Policy{Windows: c.MaintenanceWindows, Location: loc,
		TimeTrusted: system.TimeTrusted(a.Cfg.Paths.RunDir), SecurityPolicy: c.SecurityPolicy}
}

// ---- publishing and player supervision -------------------------------------------

func (a *Agent) writePlayerConfig() {
	pc := map[string]any{"url": a.PlayerURL(), "dev_image": a.Cfg.Device.DevImage}
	if err := config.WriteJSONAtomic(a.Cfg.PlayerConfigPath(), pc, 0o644); err != nil {
		a.Log.Warn("player config", "err", err)
	}
}

// publishLoop writes the runtime state for castmill-health, pushes state to
// the players and feeds the systemd watchdog.
func (a *Agent) publishLoop(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	wd := system.WatchdogInterval()
	lastWD := time.Time{}
	for {
		a.publish()
		if wd > 0 && time.Since(lastWD) >= wd {
			_ = system.Notify("WATCHDOG=1")
			lastWD = time.Now()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (a *Agent) publish() {
	conn := telemetry.ConnectedOutputs(a.SysRoot)
	attached := a.player.Outputs()
	a.mu.Lock()
	outs := map[string]state.OutputState{}
	for name, c := range conn {
		_, att := attached[name]
		outs[name] = state.OutputState{Connected: c, Player: att, LastFrame: a.player.LastFrame(name),
			GPU: a.gpuSnapshot(name)}
	}
	a.rt.Outputs = outs
	a.rt.Display = map[string]state.DisplayState{}
	for name, p := range a.displayPower {
		a.rt.Display[name] = *p
	}
	a.rt.Updated = time.Now().UTC()
	a.rt.TimeTrusted = system.TimeTrusted(a.Cfg.Paths.RunDir)
	if a.upd != nil {
		a.rt.Update = a.upd.Status()
	}
	rt := a.rt
	a.mu.Unlock()
	a.player.SetState(playerlink.PlayerState{PlayerURL: a.PlayerURL(), DevImage: a.Cfg.Device.DevImage,
		OSVersion: a.osVersion, DeviceName: rt.DeviceName, BackendError: rt.BackendError,
		Online: time.Since(rt.BackendOK) < 3*time.Minute})
	if err := config.WriteJSONAtomic(a.Cfg.RuntimeStatePath(), rt, 0o644); err != nil {
		a.Log.Warn("runtime state", "err", err)
	}
}

// frameWatchdog implements recovery layer 1 (spec 05): an output without a
// "frame rendered" heartbeat is reloaded, then its player is restarted.
func (a *Agent) frameWatchdog(ctx context.Context) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if a.Systemd == nil || time.Since(start) < 90*time.Second {
			continue // give the session time to start
		}
		timeout := a.Cfg.Player.FrameTimeout.Duration
		attached := a.player.Outputs()
		a.mu.Lock()
		disabled := map[string]bool{}
		for n, oc := range a.devcfg.Outputs {
			disabled[n] = oc.Disabled
		}
		a.mu.Unlock()
		for name, connected := range telemetry.ConnectedOutputs(a.SysRoot) {
			rs := a.recovery[name]
			if rs == nil {
				rs = &recoveryState{}
				a.recovery[name] = rs
			}
			if !connected || disabled[name] {
				*rs = recoveryState{}
				continue
			}
			unit := "castmill-player@" + name + ".service"
			last, ok := attached[name]
			if !ok {
				if rs.missingSince.IsZero() {
					rs.missingSince = time.Now()
				} else if time.Since(rs.missingSince) > 60*time.Second && time.Since(rs.restartedAt) > 2*time.Minute {
					a.Log.Warn("no player on connected output; starting it", "output", name)
					rs.restartedAt = time.Now()
					_ = a.Systemd.RestartUnit(ctx, unit)
				}
				continue
			}
			rs.missingSince = time.Time{}
			if time.Since(last) < timeout {
				rs.reloadedAt, rs.restartedAt = time.Time{}, time.Time{}
				continue
			}
			switch {
			case rs.reloadedAt.IsZero():
				a.Log.Warn("no frames; reloading player window", "output", name, "last_frame", last)
				rs.reloadedAt = time.Now()
				_ = a.player.Reload(name)
			case time.Since(rs.reloadedAt) > timeout && (rs.restartedAt.IsZero() || time.Since(rs.restartedAt) > 2*time.Minute):
				a.Log.Error("no frames after reload; restarting player", "output", name)
				rs.restartedAt = time.Now()
				if err := a.Systemd.RestartUnit(ctx, unit); err != nil {
					a.Log.Error("restart player", "err", err)
				}
			}
		}
	}
}

// ---- helpers -------------------------------------------------------------------

func osRelease(root string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(filepath.Join(root, "etc/os-release"))
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if ok {
			out[k] = strings.Trim(v, `"`)
		}
	}
	return out
}

// versionedRAUC fills in the version of a slot that was flashed at the
// factory (RAUC only records bundle versions for slots it installed).
type versionedRAUC struct {
	rauc.Client
	osVersion string
}

func (v versionedRAUC) Status(ctx context.Context) (*rauc.Status, error) {
	st, err := v.Client.Status(ctx)
	if err == nil {
		if b := st.Booted(); b != nil && b.BundleVersion == "" {
			b.BundleVersion = v.osVersion
		}
	}
	return st, err
}
