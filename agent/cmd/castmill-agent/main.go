// Command castmill-agent is the Castmill OS device agent. One binary, several
// roles (keeps the image small):
//
//	castmill-agent run         daemon (castmill-agent.service, user "agent")
//	castmill-agent health      post-boot health check, commits or rejects the slot
//	                           (castmill-health.service, root)
//	castmill-agent health --report-only   periodic check (castmill-health.timer)
//	castmill-agent provision   apply castmill.conf (castmill-provision.service, root)
//	castmill-agent session-config weston|disabled
//	                           compositor configuration for the display session
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
	"github.com/castmill/castmill-os/agent/internal/config"
	"github.com/castmill/castmill-os/agent/internal/health"
	"github.com/castmill/castmill-os/agent/internal/provision"
	"github.com/castmill/castmill-os/agent/internal/rauc"
	"github.com/castmill/castmill-os/agent/internal/session"
	"github.com/castmill/castmill-os/agent/internal/state"
	"github.com/castmill/castmill-os/agent/internal/system"
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
	case "status":
		err = printStatus(cfg)
	case "session-config":
		err = sessionConfig(cfg, fs.Arg(0))
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
	fmt.Fprintln(os.Stderr, "usage: castmill-agent run|health [--report-only]|provision|session-config weston|disabled|status|version [--config FILE]")
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

func runProvision(cfg *config.Agent, log *slog.Logger) error {
	uid, gid := -1, -1
	if u, err := user.Lookup("agent"); err == nil {
		uid, _ = strconv.Atoi(u.Uid)
		gid, _ = strconv.Atoi(u.Gid)
	}
	p := provision.Paths{
		Sources:      provision.DefaultSources(cfg.Paths.BootDir),
		NMDir:        filepath.Join(cfg.ConfigDir(), "network"),
		ConfigDir:    cfg.ConfigDir(),
		TimesyncConf: filepath.Join(cfg.ConfigDir(), "timesyncd.conf"),
		ProxyEnv:     filepath.Join(cfg.ConfigDir(), "proxy.env"),
		Localtime:    cfg.LocaltimePath(),
		ZoneinfoDir:  "/usr/share/zoneinfo",
		AgentUID:     uid, AgentGID: gid,
	}
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
