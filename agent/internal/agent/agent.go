// Package agent is the castmill-agent daemon: enrollment, backend session,
// updates, configuration, commands, telemetry and player supervision
// (spec 07).
package agent

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/castmill/castmill-os/agent/internal/api"
	"github.com/castmill/castmill-os/agent/internal/backend"
	"github.com/castmill/castmill-os/agent/internal/config"
	"github.com/castmill/castmill-os/agent/internal/identity"
	"github.com/castmill/castmill-os/agent/internal/maintenance"
	"github.com/castmill/castmill-os/agent/internal/playerlink"
	"github.com/castmill/castmill-os/agent/internal/provision"
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
	Systemd system.Manager // nil disables unit control
	SysRoot string         // "/" on a device

	id      *identity.Identity
	prov    *config.Provisioning
	client  *backend.Client
	player  *playerlink.Server
	upd     *updater.Updater
	sampler *telemetry.Sampler

	mu          sync.Mutex
	devcfg      api.DeviceConfig
	rt          state.Runtime
	osVersion   string
	hw          api.Hardware
	revoked     bool
	ws          *backend.Conn
	updateKick  chan struct{}
	recovery    map[string]*recoveryState
	statusDirty bool
	bundleCA    string // CA file RAUC uses to verify the bundle server
}

type recoveryState struct {
	reloadedAt   time.Time
	restartedAt  time.Time
	missingSince time.Time
}

// Run blocks until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) error {
	if a.SysRoot == "" {
		a.SysRoot = "/"
	}
	a.updateKick = make(chan struct{}, 1)
	a.recovery = map[string]*recoveryState{}
	for _, d := range []string{a.Cfg.Paths.RunDir, a.Cfg.StateDir(), a.Cfg.AgentConfigDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	var err error
	if a.id, err = identity.LoadOrCreate(a.Cfg.IdentityDir()); err != nil {
		return fmt.Errorf("identity: %w", err)
	}
	if a.prov, err = config.LoadProvisioning(a.Cfg.ProvisioningPath()); err != nil {
		a.Log.Warn("provisioning file unreadable", "err", err)
		a.prov = &config.Provisioning{}
	}
	_ = config.ReadJSON(a.Cfg.DeviceConfigPath(), &a.devcfg)
	a.osVersion = osRelease(a.SysRoot)["VERSION_ID"]
	a.hw = a.hardware()

	backendURL, roots, err := a.backendTarget()
	if err != nil {
		return err
	}
	if a.client, err = backend.New(backendURL, a.id.TLSConfig(roots)); err != nil {
		return err
	}
	a.mu.Lock()
	a.rt = state.Runtime{AgentVersion: Version, Fingerprint: a.id.Fingerprint(), BackendURL: backendURL,
		Enrolled: a.id.Enrolled(), DeviceID: a.id.DeviceID, Outputs: map[string]state.OutputState{}}
	a.mu.Unlock()

	if a.player, err = playerlink.Listen(a.Cfg.PlayerSocketPath(), a.Log.With("c", "player")); err != nil {
		return fmt.Errorf("player socket: %w", err)
	}
	go a.player.Serve(ctx)
	a.writePlayerConfig()

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
			InstallOpts: func() rauc.InstallOptions {
				// RAUC (root) streams the bundle over mTLS with the device identity.
				return rauc.InstallOptions{TLSCert: filepath.Join(a.Cfg.IdentityDir(), "device.crt"),
					TLSKey: filepath.Join(a.Cfg.IdentityDir(), "device.key"), TLSCA: a.bundleCA}
			},
			OnChange: func(api.UpdateStatus) { a.markDirty() },
		}
		if err := a.upd.Resume(ctx); err != nil {
			a.Log.Error("update state", "err", err)
		}
		go a.updateLoop(ctx)
	}
	a.sampler = &telemetry.Sampler{Root: a.SysRoot, DataPath: a.Cfg.Paths.DataRoot}

	go a.publishLoop(ctx)
	go a.frameWatchdog(ctx)
	_ = system.Notify("READY=1\nSTATUS=starting")

	a.backendLoop(ctx)
	return nil
}

// backendTarget returns the backend URL and pinned roots, applying the
// castmill.conf override on development images only.
func (a *Agent) backendTarget() (string, *x509.CertPool, error) {
	a.bundleCA = a.Cfg.Backend.CAFile
	if a.Cfg.Backend.AllowOverride && a.prov.BackendURL != "" {
		roots, err := identity.LoadRoots("", a.prov.BackendCA)
		if err == nil && a.prov.BackendCA != "" {
			// RAUC (streaming the bundle) needs the CA as a file.
			a.bundleCA = filepath.Join(a.Cfg.AgentConfigDir(), "backend-ca.pem")
			err = config.WriteFileAtomic(a.bundleCA, []byte(a.prov.BackendCA), 0o644)
		}
		a.Log.Warn("using the backend from castmill.conf (development image)", "url", a.prov.BackendURL)
		return a.prov.BackendURL, roots, err
	}
	roots, err := identity.LoadRoots(a.Cfg.Backend.CAFile, "")
	return a.Cfg.Backend.URL, roots, err
}

func (a *Agent) hardware() api.Hardware {
	hw := api.Hardware{Machine: a.Cfg.Device.Machine, Compatible: a.Cfg.Device.Compatible,
		OSVersion: a.osVersion, DevImage: a.Cfg.Device.DevImage}
	var h struct{ Serial, Model string }
	if err := config.ReadJSON(filepath.Join(a.Cfg.ConfigDir(), "hardware.json"), &h); err == nil {
		hw.Serial, hw.Model = h.Serial, h.Model
	}
	return hw
}

// ---- backend session -------------------------------------------------------

func (a *Agent) backendLoop(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		var err error
		if !a.id.Enrolled() {
			err = a.enroll(ctx)
		} else {
			err = a.session(ctx)
		}
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, backend.ErrUnauthorized) && a.id.Enrolled() {
			a.mu.Lock()
			a.revoked = true
			a.mu.Unlock()
			a.Log.Error("backend rejected the device certificate (revoked?); factory reset needed to re-enroll")
			backoff = 30 * time.Minute
		}
		if err != nil {
			a.setBackendError(err)
			a.Log.Warn("backend", "err", err, "retry_in", backoff)
		} else {
			backoff = time.Second
		}
		jitter := time.Duration(rand.Int64N(int64(backoff)/4 + 1))
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff + jitter):
		}
		backoff = min(backoff*2, 5*time.Minute)
		if a.isRevoked() {
			backoff = 30 * time.Minute
		}
	}
}

func (a *Agent) enroll(ctx context.Context) error {
	csr, err := a.id.CSR()
	if err != nil {
		return err
	}
	pendPath := filepath.Join(a.Cfg.StateDir(), "enrollment.json")
	var e api.Enrollment
	if config.ReadJSON(pendPath, &e) != nil || e.ID == "" || (!e.ExpiresAt.IsZero() && time.Now().After(e.ExpiresAt)) {
		req := api.EnrollmentRequest{CSR: csr, Hardware: a.hw, OrgToken: a.prov.OrgToken, Group: a.prov.Group,
			Name: expandName(a.prov.NameTemplate, a.prov.Group, a.hw.Serial, a.id.Fingerprint())}
		ne, err := a.client.Enroll(ctx, req)
		if err != nil {
			return err
		}
		e = *ne
		a.backendOK()
		_ = config.WriteJSONAtomic(pendPath, e, 0o600)
		a.Log.Info("enrollment started", "id", e.ID, "pairing_code", e.PairingCode)
	}
	for {
		switch e.Status {
		case api.EnrollmentApproved:
			if err := a.id.Store(e.DeviceID, e.Certificate); err != nil {
				return fmt.Errorf("storing certificate: %w", err)
			}
			_ = os.Remove(pendPath)
			a.client.ResetConnections()
			a.mu.Lock()
			a.rt.Enrolled, a.rt.DeviceID, a.rt.PairingCode = true, e.DeviceID, ""
			a.mu.Unlock()
			a.Log.Info("enrolled", "device_id", e.DeviceID)
			return nil
		case api.EnrollmentRejected, api.EnrollmentExpired:
			_ = os.Remove(pendPath)
			return fmt.Errorf("enrollment %s: %s %s", e.ID, e.Status, e.Reason)
		}
		a.mu.Lock()
		a.rt.PairingCode = e.PairingCode
		a.mu.Unlock()
		wait := time.Duration(max(e.PollIntervalS, 2)) * time.Second
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		ne, err := a.client.Enrollment(ctx, e.ID)
		if err != nil {
			var he *backend.HTTPError
			if errors.As(err, &he) && he.Status == 404 {
				_ = os.Remove(pendPath)
			}
			return err
		}
		a.backendOK()
		pc := e.PairingCode
		e = *ne
		if e.PairingCode == "" {
			e.PairingCode = pc
		}
	}
}

func expandName(tmpl, group, serial, fp string) string {
	if tmpl == "" {
		return ""
	}
	if serial == "" {
		serial = fp
	}
	r := strings.NewReplacer("{group}", strings.ReplaceAll(group, "/", "-"), "{serial}", serial)
	return r.Replace(tmpl)
}

// session runs one connected period: sync, then WebSocket until it drops.
func (a *Agent) session(ctx context.Context) error {
	devID := a.id.DeviceID
	if a.id.NeedsRenewal(time.Now()) {
		if err := a.renew(ctx); err != nil {
			a.Log.Warn("certificate renewal failed", "err", err)
		}
	}
	if cfg, err := a.client.Config(ctx, devID); err != nil {
		return err
	} else {
		a.backendOK()
		a.applyConfig(ctx, cfg)
	}
	if err := a.pollDesired(ctx); err != nil {
		return err
	}
	if err := a.postStatus(ctx); err != nil {
		return err
	}

	ws, err := a.client.Dial(ctx, devID)
	if err != nil {
		return err
	}
	defer ws.Close()
	a.mu.Lock()
	a.ws, a.rt.Connected = ws, true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.ws, a.rt.Connected = nil, false
		a.mu.Unlock()
	}()
	st := a.status(ctx)
	if err := ws.Send(ctx, &api.Message{Type: api.MsgHello, Status: st}); err != nil {
		return err
	}
	a.Log.Info("connected to backend", "device_id", devID)

	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errc := make(chan error, 2)
	go func() {
		for {
			m, err := ws.Receive(sctx)
			if err != nil {
				errc <- err
				return
			}
			a.backendOK()
			go a.handle(sctx, ws, m)
		}
	}()

	ping := time.NewTicker(30 * time.Second)
	tel := time.NewTicker(a.telemetryInterval())
	stat := time.NewTicker(a.Cfg.Intervals.Status.Duration)
	des := time.NewTicker(a.Cfg.Intervals.Desired.Duration)
	dirty := time.NewTicker(5 * time.Second)
	defer ping.Stop()
	defer tel.Stop()
	defer stat.Stop()
	defer des.Stop()
	defer dirty.Stop()
	a.sendTelemetry(sctx, ws)
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errc:
			return fmt.Errorf("websocket: %w", err)
		case <-ping.C:
			if err := ws.Ping(sctx); err != nil {
				return fmt.Errorf("websocket ping: %w", err)
			}
			a.backendOK()
		case <-tel.C:
			a.sendTelemetry(sctx, ws)
		case <-stat.C:
			a.markDirty()
		case <-dirty.C:
			if a.takeDirty() {
				if err := a.postStatus(sctx); err != nil {
					a.Log.Warn("status", "err", err)
				}
			}
		case <-des.C:
			if err := a.pollDesired(sctx); err != nil {
				a.Log.Warn("desired", "err", err)
			}
		}
	}
}

func (a *Agent) renew(ctx context.Context) error {
	csr, err := a.id.CSR()
	if err != nil {
		return err
	}
	cert, err := a.client.RenewCertificate(ctx, a.id.DeviceID, csr)
	if err != nil {
		return err
	}
	if err := a.id.Store(a.id.DeviceID, cert); err != nil {
		return err
	}
	a.client.ResetConnections()
	return nil
}

func (a *Agent) pollDesired(ctx context.Context) error {
	d, err := a.client.Desired(ctx, a.id.DeviceID)
	if err != nil {
		return err
	}
	a.backendOK()
	if a.upd != nil {
		a.upd.SetDesired(d)
		a.kickUpdate()
	}
	return nil
}

func (a *Agent) postStatus(ctx context.Context) error {
	if err := a.client.PostStatus(ctx, a.id.DeviceID, a.status(ctx)); err != nil {
		return err
	}
	a.backendOK()
	if a.upd != nil {
		a.upd.MarkReported()
	}
	return nil
}

func (a *Agent) status(ctx context.Context) *api.DeviceStatus {
	st := &api.DeviceStatus{OSVersion: a.osVersion, AgentVersion: Version, Machine: a.hw.Machine,
		Compatible: a.hw.Compatible, TimeTrusted: system.TimeTrusted(a.Cfg.Paths.RunDir),
		Uptime: telemetry.Uptime(a.SysRoot)}
	if a.RAUC != nil {
		if rs, err := a.RAUC.Status(ctx); err == nil {
			st.Slots = rs.API()
			if b := rs.Booted(); b != nil {
				st.BootedSlot = b.Bootname
				if a.upd != nil {
					a.upd.RefreshCommit(b.Bootname)
				}
			}
		}
	}
	if a.upd != nil {
		st.Update = a.upd.Status()
	}
	var rep api.HealthReport
	if config.ReadJSON(a.Cfg.HealthReportPath(), &rep) == nil {
		st.Health = &rep
	}
	if per := a.periodicHealth(); per != nil && (st.Health == nil || per.Time.After(st.Health.Time)) {
		st.Health = per
	}
	a.mu.Lock()
	for name, o := range a.rt.Outputs {
		st.Outputs = append(st.Outputs, api.Output{Name: name, Connected: o.Connected, LastFrame: o.LastFrame})
	}
	a.mu.Unlock()
	sort.Slice(st.Outputs, func(i, j int) bool { return st.Outputs[i].Name < st.Outputs[j].Name })
	st.DecodeCaps = a.player.Capabilities()
	return st
}

func (a *Agent) periodicHealth() *api.HealthReport {
	var rep api.HealthReport
	if config.ReadJSON(filepath.Join(a.Cfg.Paths.RunDir, "health-periodic.json"), &rep) != nil {
		return nil
	}
	return &rep
}

func (a *Agent) sendTelemetry(ctx context.Context, ws *backend.Conn) {
	t := a.sampler.Sample()
	t.Player = a.player.Stats()
	if err := ws.Send(ctx, &api.Message{Type: api.MsgTelemetry, Telemetry: t}); err != nil {
		a.Log.Warn("telemetry", "err", err)
	}
}

func (a *Agent) telemetryInterval() time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.devcfg.TelemetryIntervalS > 0 {
		return time.Duration(a.devcfg.TelemetryIntervalS) * time.Second
	}
	return a.Cfg.Intervals.Telemetry.Duration
}

// ---- commands ----------------------------------------------------------------

func (a *Agent) handle(ctx context.Context, ws *backend.Conn, m *api.Message) {
	switch m.Type {
	case api.MsgConfig:
		if m.Config != nil {
			a.applyConfig(ctx, m.Config)
		}
	case api.MsgDesired:
		if m.Desired != nil && a.upd != nil {
			a.upd.SetDesired(m.Desired)
			a.kickUpdate()
		} else if err := a.pollDesired(ctx); err != nil {
			a.Log.Warn("desired", "err", err)
		}
	case api.MsgCommand:
		a.Log.Info("command", "id", m.ID, "command", m.Command)
		data, err := a.command(ctx, m)
		res := &api.Message{Type: api.MsgResult, ID: m.ID, OK: err == nil, Data: data}
		if err != nil {
			res.Error = err.Error()
			a.Log.Warn("command failed", "id", m.ID, "command", m.Command, "err", err)
		}
		if err := ws.Send(ctx, res); err != nil {
			a.Log.Warn("sending result", "err", err)
		}
		if err == nil && (m.Command == api.CmdReboot || m.Command == api.CmdFactoryReset) {
			time.Sleep(2 * time.Second) // let the result reach the backend
			if a.Systemd != nil {
				if err := a.Systemd.Reboot(context.Background()); err != nil {
					a.Log.Error("reboot", "err", err)
				}
			}
		}
	}
}

func argString(m *api.Message, k string) string {
	if v, ok := m.Args[k].(string); ok {
		return v
	}
	return ""
}

func (a *Agent) command(ctx context.Context, m *api.Message) (map[string]any, error) {
	switch m.Command {
	case api.CmdReboot:
		if a.Systemd == nil {
			return nil, errors.New("reboot not available")
		}
		return nil, nil
	case api.CmdRestartPlayer:
		if a.Systemd == nil {
			return nil, errors.New("unit control not available")
		}
		unit := "castmill-session.service"
		if o := argString(m, "output"); o != "" {
			unit = "castmill-player@" + o + ".service"
		}
		return map[string]any{"unit": unit}, a.Systemd.RestartUnit(ctx, unit)
	case api.CmdReloadPlayer:
		outs := a.targetOutputs(argString(m, "output"))
		for _, o := range outs {
			if err := a.player.Reload(o); err != nil {
				return nil, err
			}
		}
		return map[string]any{"outputs": outs}, nil
	case api.CmdScreenshot:
		outs := a.targetOutputs(argString(m, "output"))
		if len(outs) == 0 {
			return nil, errors.New("no player attached")
		}
		ids := map[string]any{}
		for _, o := range outs {
			sctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			png, err := a.player.Screenshot(sctx, o)
			cancel()
			if err != nil {
				return nil, fmt.Errorf("%s: %w", o, err)
			}
			id, err := a.client.Upload(ctx, a.id.DeviceID, "screenshot", m.ID, "image/png", bytes.NewReader(png))
			if err != nil {
				return nil, err
			}
			ids[o] = id
		}
		return map[string]any{"uploads": ids}, nil
	case api.CmdCollectLogs:
		blob, err := collectLogs(ctx)
		if err != nil {
			return nil, err
		}
		id, err := a.client.Upload(ctx, a.id.DeviceID, "logs", m.ID, "application/gzip", bytes.NewReader(blob))
		if err != nil {
			return nil, err
		}
		return map[string]any{"upload": id, "bytes": len(blob)}, nil
	case api.CmdFactoryReset:
		// The backend asks the operator to confirm in the UI and sends the
		// device ID back as confirmation.
		if argString(m, "confirm") != a.id.DeviceID {
			return nil, errors.New("factory reset needs confirm=<device id>")
		}
		if a.Systemd == nil {
			return nil, errors.New("reboot not available")
		}
		return nil, system.RequestFactoryReset(a.Cfg.StateDir())
	case api.CmdCheckUpdate:
		return nil, a.pollDesired(ctx)
	case api.CmdApplyNow:
		if a.upd == nil {
			return nil, errors.New("updates not available")
		}
		if err := a.upd.ForceActivate(); err != nil {
			return nil, err
		}
		a.kickUpdate()
		return nil, nil
	}
	return nil, fmt.Errorf("unknown command %q", m.Command)
}

func (a *Agent) targetOutputs(only string) []string {
	if only != "" {
		return []string{only}
	}
	var outs []string
	for o := range a.player.Outputs() {
		outs = append(outs, o)
	}
	sort.Strings(outs)
	return outs
}

// collectLogs returns the journal of the current and previous boot, gzipped.
// Secrets are never logged by Castmill components (spec 07).
func collectLogs(ctx context.Context) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	for _, boot := range []string{"-1", "0"} {
		cmd := exec.CommandContext(ctx, "journalctl", "--no-pager", "-o", "short-iso-precise", "-b", boot, "-n", "50000")
		cmd.Stdout = zw
		fmt.Fprintf(zw, "===== journal boot %s =====\n", boot)
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(zw, "(journalctl: %v)\n", err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---- configuration -----------------------------------------------------------

func (a *Agent) applyConfig(ctx context.Context, c *api.DeviceConfig) {
	if err := maintenance.Validate(c.MaintenanceWindows); err != nil {
		a.Log.Warn("ignoring invalid maintenance windows", "err", err)
		c.MaintenanceWindows = nil
	}
	a.mu.Lock()
	old := a.devcfg
	a.devcfg = *c
	a.mu.Unlock()
	if reflect.DeepEqual(old, *c) {
		return
	}
	if err := config.WriteJSONAtomic(a.Cfg.DeviceConfigPath(), c, 0o644); err != nil {
		a.Log.Error("saving device config", "err", err)
	}
	if c.Timezone != "" && c.Timezone != old.Timezone {
		if err := provision.SetTimezone(filepath.Join(a.SysRoot, "usr/share/zoneinfo"), a.Cfg.LocaltimePath(), c.Timezone); err != nil {
			a.Log.Warn("timezone", "err", err)
		}
	}
	a.writePlayerConfig()
	if !reflect.DeepEqual(old.Outputs, c.Outputs) {
		if err := config.WriteJSONAtomic(a.Cfg.DisplayConfigPath(), c.Outputs, 0o644); err != nil {
			a.Log.Error("saving display config", "err", err)
		} else if a.Systemd != nil && old.Revision != 0 {
			// Weston has no runtime output IPC: re-create the session (ADR-0004).
			a.Log.Info("display configuration changed; restarting the session")
			if err := a.Systemd.RestartUnit(ctx, "castmill-session.service"); err != nil {
				a.Log.Warn("restart session", "err", err)
			}
		}
	}
	a.Log.Info("configuration applied", "revision", c.Revision)
}

func (a *Agent) playerURL() string {
	a.mu.Lock()
	u := a.devcfg.PlayerURL
	a.mu.Unlock()
	if u == "" {
		u = a.Cfg.Player.DefaultURL
	}
	return strings.ReplaceAll(u, "{device_id}", a.id.DeviceID)
}

func (a *Agent) writePlayerConfig() {
	pc := map[string]any{"url": a.playerURL(), "dev_image": a.Cfg.Device.DevImage}
	if err := config.WriteJSONAtomic(a.Cfg.PlayerConfigPath(), pc, 0o644); err != nil {
		a.Log.Warn("player config", "err", err)
	}
}

func (a *Agent) policy() maintenance.Policy {
	a.mu.Lock()
	c := a.devcfg
	a.mu.Unlock()
	loc := time.Local
	if c.Timezone != "" {
		if l, err := time.LoadLocation(c.Timezone); err == nil {
			loc = l
		}
	}
	return maintenance.Policy{Windows: c.MaintenanceWindows, Location: loc,
		TimeTrusted: system.TimeTrusted(a.Cfg.Paths.RunDir), SecurityPolicy: c.SecurityPolicy}
}

// ---- background loops ----------------------------------------------------------

func (a *Agent) kickUpdate() {
	select {
	case a.updateKick <- struct{}{}:
	default:
	}
}

func (a *Agent) updateLoop(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		if err := a.upd.Step(ctx); err != nil {
			a.Log.Warn("update step", "err", err)
		}
		a.markDirty()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-a.updateKick:
		}
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
	trusted := system.TimeTrusted(a.Cfg.Paths.RunDir)
	if !trusted && !a.client.ServerTime().IsZero() {
		if err := system.SetClockFromBackend(a.Cfg.Paths.RunDir, a.client.ServerTime()); err != nil {
			a.Log.Debug("clock from backend", "err", err)
		}
	}
	a.mu.Lock()
	outs := map[string]state.OutputState{}
	for name, c := range conn {
		_, att := attached[name]
		outs[name] = state.OutputState{Connected: c, Player: att, LastFrame: a.player.LastFrame(name)}
	}
	a.rt.Outputs = outs
	a.rt.Updated = time.Now().UTC()
	a.rt.TimeTrusted = trusted
	a.rt.Enrolled = a.id.Enrolled()
	a.rt.DeviceID = a.id.DeviceID
	if a.upd != nil {
		a.rt.Update = a.upd.Status()
	}
	rt := a.rt
	ps := playerlink.PlayerState{Enrolled: rt.Enrolled, DeviceID: rt.DeviceID, Fingerprint: rt.Fingerprint,
		PairingCode: rt.PairingCode, DevImage: a.Cfg.Device.DevImage, OSVersion: a.osVersion,
		BackendError: rt.BackendError, Online: time.Since(rt.BackendOK) < 2*time.Minute}
	if a.revoked {
		ps.BackendError = "This device was revoked in Castmill. A factory reset is needed to enroll it again."
	}
	a.mu.Unlock()
	if rt.Enrolled {
		ps.PlayerURL = a.playerURL()
	}
	a.player.SetState(ps)
	if err := config.WriteJSONAtomic(a.Cfg.RuntimeStatePath(), rt, 0o644); err != nil {
		a.Log.Warn("runtime state", "err", err)
	}
	status := "pairing"
	if rt.Enrolled {
		status = "enrolled, backend " + map[bool]string{true: "connected", false: "disconnected"}[rt.Connected]
	}
	_ = system.Notify("STATUS=" + status)
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

func (a *Agent) backendOK() {
	a.mu.Lock()
	a.rt.BackendOK, a.rt.BackendError = time.Now().UTC(), ""
	a.revoked = false
	a.mu.Unlock()
}

func (a *Agent) setBackendError(err error) {
	a.mu.Lock()
	a.rt.BackendError = err.Error()
	a.mu.Unlock()
}

func (a *Agent) isRevoked() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.revoked
}

func (a *Agent) markDirty() {
	a.mu.Lock()
	a.statusDirty = true
	a.mu.Unlock()
}

func (a *Agent) takeDirty() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	d := a.statusDirty
	a.statusDirty = false
	return d
}

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
