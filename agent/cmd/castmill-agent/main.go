// Command castmill-agent is the Castmill OS device agent. One binary, several
// roles (keeps the image small):
//
//	castmill-agent run         daemon (castmill-agent.service, user "agent")
//	castmill-agent health      post-boot health check, commits or rejects the slot
//	                           (castmill-health.service, root)
//	castmill-agent health --report-only   periodic check (castmill-health.timer)
//	castmill-agent provision   apply castmill.conf (castmill-provision.service, root)
//	castmill-agent provision-usb /dev/PARTITION
//	                           apply castmill.conf from a USB stick while offline
//	                           (castmill-usb-provision@.service, root, started by udev)
//	castmill-agent update URL VERSION [--now]
//	                           install an OS update bundle (development and support;
//	                           Castmill-driven OS updates come with the backend support)
//	castmill-agent session-config weston|disabled
//	                           compositor configuration for the display session
//	castmill-agent display OUTPUT on|off
//	                           switch a display over HDMI-CEC or DDC/CI (root; for
//	                           checking hardware: the device schedule does this itself)
//	castmill-agent status      print the agent state (for support / SSH on dev images)
//	castmill-agent version
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/castmill/castmill-os/agent/internal/agent"
	"github.com/castmill/castmill-os/agent/internal/api"
	"github.com/castmill/castmill-os/agent/internal/config"
	"github.com/castmill/castmill-os/agent/internal/display"
	"github.com/castmill/castmill-os/agent/internal/health"
	"github.com/castmill/castmill-os/agent/internal/provision"
	"github.com/castmill/castmill-os/agent/internal/rauc"
	"github.com/castmill/castmill-os/agent/internal/session"
	"github.com/castmill/castmill-os/agent/internal/state"
	"github.com/castmill/castmill-os/agent/internal/system"
	"github.com/castmill/castmill-os/agent/internal/wifi"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmd, args := os.Args[1], os.Args[2:]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath, "agent configuration file")
	verbose := fs.Bool("v", false, "debug logging")
	reportOnly := fs.Bool("report-only", false, "health: never mark slots or reboot")
	now := fs.Bool("now", false, "update: activate right after installing (ignore the maintenance window)")
	_ = fs.Parse(args)

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	// journald adds timestamps; keep lines short.
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && os.Getenv("JOURNAL_STREAM") != "" {
				return slog.Attr{}
			}
			return a
		}}))

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fatal(log, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	switch cmd {
	case "run":
		err = run(ctx, cfg, log)
	case "health":
		err = runHealth(ctx, cfg, log, *reportOnly)
	case "provision":
		err = runProvision(cfg, log)
	case "provision-usb":
		err = runProvisionUSB(cfg, log, fs.Arg(0))
	case "status":
		err = printStatus(cfg)
	case "session-config":
		err = sessionConfig(cfg, fs.Arg(0))
	case "update":
		err = requestUpdate(cfg, fs.Arg(0), fs.Arg(1), *now)
	case "display":
		err = switchDisplay(fs.Arg(0), fs.Arg(1))
	case "version":
		fmt.Println(agent.Version)
	default:
		usage()
	}
	if err != nil {
		fatal(log, err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: castmill-agent run|health [--report-only]|provision|update URL VERSION [--now]|display OUTPUT on|off|session-config weston|disabled|status|version [--config FILE]")
	os.Exit(2)
}

func fatal(log *slog.Logger, err error) {
	log.Error(err.Error())
	os.Exit(1)
}

func run(ctx context.Context, cfg *config.Agent, log *slog.Logger) error {
	a := &agent.Agent{Cfg: cfg, Log: log}
	if r, err := rauc.Connect(); err != nil {
		log.Warn("RAUC not available; updates disabled", "err", err)
	} else {
		defer r.Close()
		a.RAUC = r
	}
	if m, err := system.NewManager(); err != nil {
		log.Warn("systemd D-Bus not available; unit control disabled", "err", err)
	} else {
		a.Systemd = m
	}
	if w, err := wifi.Connect(); err != nil {
		log.Warn("NetworkManager not available; no on-screen Wi-Fi setup", "err", err)
	} else {
		defer w.Close()
		a.WiFi = w
	}
	return a.Run(ctx)
}

func runHealth(ctx context.Context, cfg *config.Agent, log *slog.Logger, reportOnly bool) error {
	m, err := system.NewManager()
	if err != nil {
		return err
	}
	c := &health.Checker{Cfg: cfg, Systemd: m, Log: log, SysRoot: "/"}
	if r, err := rauc.Connect(); err != nil {
		log.Warn("RAUC not available; health check is report-only", "err", err)
	} else if _, err := r.Status(ctx); err != nil {
		log.Warn("RAUC has no slot status; health check is report-only", "err", err)
		r.Close()
	} else {
		defer r.Close()
		c.RAUC = r
	}
	out, err := c.Run(ctx, reportOnly)
	if err != nil {
		return err
	}
	log.Info("health", "result", out.Report.Result, "trial", out.Report.Trial, "committed", out.Report.Committed,
		"slot", out.Report.Slot, "version", out.Report.Version)
	for _, ch := range out.Report.Checks {
		log.Info("check", "name", ch.Name, "ok", ch.OK, "detail", ch.Detail)
	}
	if out.NeedReboot {
		log.Error("rebooting into the previous slot")
		return m.Reboot(ctx)
	}
	return nil
}

func provisionPaths(cfg *config.Agent, sources []string) provision.Paths {
	uid, gid := -1, -1
	if u, err := user.Lookup("agent"); err == nil {
		uid, _ = strconv.Atoi(u.Uid)
		gid, _ = strconv.Atoi(u.Gid)
	}
	return provision.Paths{
		Sources:      sources,
		NMDir:        filepath.Join(cfg.ConfigDir(), "network"),
		ConfigDir:    cfg.ConfigDir(),
		TimesyncConf: filepath.Join(cfg.ConfigDir(), "timesyncd.conf"),
		ProxyEnv:     filepath.Join(cfg.ConfigDir(), "proxy.env"),
		Localtime:    cfg.LocaltimePath(),
		ZoneinfoDir:  "/usr/share/zoneinfo",
		AgentUID:     uid, AgentGID: gid,
	}
}

func runProvision(cfg *config.Agent, log *slog.Logger) error {
	p := provisionPaths(cfg, provision.DefaultSources(cfg.Paths.BootDir))
	uid, gid := p.AgentUID, p.AgentGID
	if err := recordHardware(cfg); err != nil {
		log.Warn("hardware info", "err", err)
	}
	if _, err := os.Lstat(p.Localtime); os.IsNotExist(err) {
		if err := provision.SetTimezone(p.ZoneinfoDir, p.Localtime, "UTC"); err != nil {
			log.Warn("default timezone", "err", err)
		}
		_ = os.Lchown(p.Localtime, uid, gid)
	}
	res, err := provision.Apply(p, cfg.Backend.AllowOverride, log)
	// Every boot, before NetworkManager: castmill.conf or the time zone
	// (which Castmill can change) may have changed it.
	if cc := provision.ApplyWiFiCountry(p, wifiCountryConf(cfg), log); cc != "" {
		log.Info("Wi-Fi country", "country", cc)
	}
	if err != nil {
		return err
	}
	if res == nil {
		log.Info("no castmill.conf found")
		return nil
	}
	log.Info("provisioning applied", "source", res.Source, "applied", strings.Join(res.Applied, ","))
	return nil
}

// wifiCountryConf: the cfg80211 option, linked from
// /etc/modprobe.d/castmill-wifi-country.conf (castmill-base).
func wifiCountryConf(cfg *config.Agent) string {
	return filepath.Join(cfg.ConfigDir(), "wifi-country.conf")
}

// recordHardware stores serial and model where the unprivileged agent can
// read them (DMI serials are root-only).
func recordHardware(cfg *config.Agent) error {
	read := func(paths ...string) string {
		for _, p := range paths {
			if b, err := os.ReadFile(p); err == nil {
				if s := strings.TrimSpace(strings.TrimRight(string(b), "\x00")); s != "" &&
					!strings.EqualFold(s, "To be filled by O.E.M.") && !strings.EqualFold(s, "Default string") {
					return s
				}
			}
		}
		return ""
	}
	hw := map[string]string{
		"serial": read("/sys/class/dmi/id/product_serial", "/sys/firmware/devicetree/base/serial-number", "/sys/class/dmi/id/product_uuid"),
		"model":  read("/sys/firmware/devicetree/base/model", "/sys/class/dmi/id/product_name"),
	}
	return config.WriteJSONAtomic(filepath.Join(cfg.ConfigDir(), "hardware.json"), hw, 0o644)
}

func printStatus(cfg *config.Agent) error {
	out := map[string]any{}
	if rt, err := state.LoadRuntime(cfg.RuntimeStatePath()); err == nil {
		out["agent"] = rt
	} else {
		out["agent"] = "not running (" + err.Error() + ")"
	}
	out["update"] = state.LoadUpdate(filepath.Join(cfg.StateDir(), "update.json"))
	var rep any
	if config.ReadJSON(cfg.HealthReportPath(), &rep) == nil {
		out["health"] = rep
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// sessionConfig prints the compositor configuration (run by the session
// script as the player user).
func sessionConfig(cfg *config.Agent, what string) error {
	display := session.LoadDisplay(cfg.DisplayConfigPath())
	switch what {
	case "weston":
		fmt.Print(session.WestonINI(session.Connectors("/"), display))
	case "disabled":
		for _, o := range session.Disabled(display) {
			fmt.Println(o)
		}
	default:
		return fmt.Errorf("session-config: unknown format %q (weston, disabled)", what)
	}
	return nil
}

// requestUpdate records the desired OS version; the running agent installs
// it in the background and activates it in the maintenance window (or right
// away with --now).
func requestUpdate(cfg *config.Agent, url, version string, now bool) error {
	if url == "" || version == "" {
		return fmt.Errorf("usage: castmill-agent update URL VERSION [--now]")
	}
	d := api.Desired{Version: version, BundleURL: url, ActivatePolicy: api.ActivateWindow}
	if now {
		d.ActivatePolicy = api.ActivateNow
	}
	if err := config.WriteJSONAtomic(agent.DesiredPath(cfg), d, 0o644); err != nil {
		return err
	}
	fmt.Printf("requested update to %s (%s); follow it with: castmill-agent status\n", version, d.ActivatePolicy)
	return nil
}

func switchDisplay(output, power string) error {
	if output == "" || (power != "on" && power != "off") {
		return fmt.Errorf("usage: castmill-agent display OUTPUT on|off")
	}
	c := &display.Controller{SysRoot: "/", DevRoot: "/dev"}
	method, err := c.SetPower(output, power == "on")
	if err != nil {
		return err
	}
	fmt.Printf("%s: %s (%s)\n", output, power, method)
	return nil
}
