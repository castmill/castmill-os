// Package health implements castmill-health (spec 05, REL-002).
//
// Trial boot: the agent activated a new slot (update state "activating" with
// target == booted slot). The checks must all pass within the timeout; then
// the slot is committed with mark-good. Otherwise mark-bad + reboot, and the
// bootloader falls back to the previous slot.
//
// Normal boot: the booted slot is marked good straight away (this also resets
// the bootloader's try counter) and the checks run report-only. A committed
// slot is never rejected by the health check.
package health

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/castmill/castmill-os/agent/internal/api"
	"github.com/castmill/castmill-os/agent/internal/config"
	"github.com/castmill/castmill-os/agent/internal/rauc"
	"github.com/castmill/castmill-os/agent/internal/state"
	"github.com/castmill/castmill-os/agent/internal/system"
	"github.com/castmill/castmill-os/agent/internal/telemetry"
)

// Freshness limits.
const (
	frameMaxAge   = 60 * time.Second
	stateMaxAge   = 60 * time.Second
	backendMaxAge = 10 * time.Minute
)

// Checker runs the checks. Fields are injectable for tests.
type Checker struct {
	Cfg     *config.Agent
	RAUC    rauc.Client
	Systemd system.Manager
	Log     *slog.Logger
	// SysRoot is "/" on a device.
	SysRoot string
	// Now defaults to time.Now.
	Now func() time.Time
	// Interval between attempts during a trial boot.
	Interval time.Duration
}

func (c *Checker) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Check evaluates every item once.
func (c *Checker) Check(ctx context.Context) []api.HealthCheck {
	var out []api.HealthCheck
	add := func(name string, ok bool, format string, a ...any) {
		out = append(out, api.HealthCheck{Name: name, OK: ok, Detail: fmt.Sprintf(format, a...)})
	}

	// 1. No failed critical units.
	var bad []string
	for _, u := range c.Cfg.Health.CriticalUnits {
		st, err := c.Systemd.UnitActive(ctx, u)
		if err != nil {
			bad = append(bad, u+"("+err.Error()+")")
		} else if st != "active" && st != "reloading" {
			bad = append(bad, u+"="+st)
		}
	}
	failed, _ := c.Systemd.FailedUnits(ctx)
	add("units", len(bad) == 0, "critical: %s; failed (any): %s", orNone(bad), orNone(failed))

	// 2. data mounted and writable.
	probe := filepath.Join(c.Cfg.Paths.DataRoot, ".health-probe")
	err := os.WriteFile(probe, []byte("ok"), 0o600)
	if err == nil {
		err = os.Remove(probe)
	}
	if err == nil && !isMountPoint(c.SysRoot, c.Cfg.Paths.DataRoot) {
		err = fmt.Errorf("%s is not a mount point", c.Cfg.Paths.DataRoot)
	}
	add("data", err == nil, "%v", errOrOK(err))

	// Agent runtime state (frames, backend reachability).
	rt, rtErr := state.LoadRuntime(c.Cfg.RuntimeStatePath())
	if rtErr == nil && c.now().Sub(rt.Updated) > stateMaxAge {
		rtErr = fmt.Errorf("agent state is stale (%s)", rt.Updated.Format(time.RFC3339))
	}
	add("agent", rtErr == nil, "%v", errOrOK(rtErr))

	// 3. Compositor running with at least one output.
	outs := telemetry.ConnectedOutputs(c.SysRoot)
	var connected []string
	for name, ok := range outs {
		if ok {
			connected = append(connected, name)
		}
	}
	sort.Strings(connected)
	if c.Cfg.Health.RequireOutput {
		add("outputs", len(connected) > 0, "connected: %s", orNone(connected))
	}

	// 4. Frame rendered on every connected output.
	var stale []string
	for _, o := range connected {
		if rt == nil || rtErr != nil {
			stale = append(stale, o)
			continue
		}
		if ost := rt.Outputs[o]; c.now().Sub(ost.LastFrame) > frameMaxAge {
			stale = append(stale, o)
		}
	}
	add("frames", len(stale) == 0 && (len(connected) > 0 || !c.Cfg.Health.RequireOutput),
		"no recent frame on: %s", orNone(stale))

	// 5. Backend reached, or the network is down for reasons outside the device.
	switch {
	case rt != nil && rtErr == nil && c.now().Sub(rt.BackendOK) <= backendMaxAge:
		add("backend", true, "reached at %s", rt.BackendOK.Format(time.RFC3339))
	default:
		down, why := NetworkDown(c.SysRoot)
		detail := "backend not reached"
		if rt != nil && rt.BackendError != "" {
			detail += ": " + rt.BackendError
		}
		if down {
			add("backend", true, "network down outside the device (%s); %s", why, detail)
		} else {
			add("backend", false, "%s (network is up: %s)", detail, why)
		}
	}
	return out
}

// AllOK reports whether every check passed.
func AllOK(cs []api.HealthCheck) bool {
	for _, c := range cs {
		if !c.OK {
			return false
		}
	}
	return true
}

// Outcome of Run.
type Outcome struct {
	Report     api.HealthReport
	NeedReboot bool
}

// Run performs the post-boot health check. reportOnly never marks slots.
func (c *Checker) Run(ctx context.Context, reportOnly bool) (*Outcome, error) {
	if c.RAUC == nil {
		// No A/B updates on this platform yet (e.g. Raspberry Pi 5 until the
		// boot mechanism is decided): checks are informational only.
		rep := api.HealthReport{Checks: c.Check(ctx), Time: c.now().UTC()}
		rep.Result = result(rep.Checks)
		return &Outcome{Report: rep}, c.save(rep, reportOnly)
	}
	st, err := c.RAUC.Status(ctx)
	if err != nil {
		return nil, err
	}
	booted := st.Booted()
	if booted == nil {
		return nil, fmt.Errorf("cannot determine the booted slot (BootSlot=%q)", st.BootSlot)
	}
	upd := state.LoadUpdate(filepath.Join(c.Cfg.StateDir(), "update.json"))
	trial := upd.State == api.UpdateActivating && upd.TargetSlot == booted.Bootname
	rep := api.HealthReport{Slot: booted.Bootname, Version: booted.BundleVersion, Trial: trial}

	if reportOnly || !trial {
		if !reportOnly {
			if err := c.RAUC.Mark(ctx, "good", "booted"); err != nil {
				c.Log.Error("mark-good failed", "err", err)
			}
			rep.Committed = true
			// The new slot never booted (bootloader fell back): make sure it
			// isn't tried again.
			if upd.State == api.UpdateActivating && upd.TargetSlot != booted.Bootname {
				if err := c.RAUC.Mark(ctx, "bad", "other"); err != nil {
					c.Log.Warn("mark-bad other failed", "err", err)
				}
			}
		}
		rep.Checks = c.Check(ctx)
		rep.Result = result(rep.Checks)
		rep.Time = c.now().UTC()
		return &Outcome{Report: rep}, c.save(rep, reportOnly)
	}

	// Trial boot: wait for all checks to pass.
	deadline := c.now().Add(c.Cfg.Health.Timeout.Duration)
	interval := c.Interval
	if interval == 0 {
		interval = 10 * time.Second
	}
	c.Log.Info("trial boot: waiting for health checks", "slot", booted.Bootname, "version", booted.BundleVersion,
		"timeout", c.Cfg.Health.Timeout.Duration)
	for {
		rep.Checks = c.Check(ctx)
		rep.Time = c.now().UTC()
		if AllOK(rep.Checks) {
			if err := c.RAUC.Mark(ctx, "good", "booted"); err != nil {
				// Retry: committing is essential, the next attempt may work.
				c.Log.Error("mark-good failed", "err", err)
			} else {
				rep.Result, rep.Committed = "pass", true
				c.Log.Info("slot committed", "slot", booted.Bootname)
				return &Outcome{Report: rep}, c.save(rep, false)
			}
		}
		rep.Result = "pending"
		_ = c.save(rep, false)
		if !c.now().Before(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
	rep.Result = "fail"
	c.Log.Error("health check failed; rejecting slot", "slot", booted.Bootname, "checks", failedNames(rep.Checks))
	if err := c.save(rep, false); err != nil {
		c.Log.Error("saving health report", "err", err)
	}
	if err := c.RAUC.Mark(ctx, "bad", "booted"); err != nil {
		return &Outcome{Report: rep}, err
	}
	return &Outcome{Report: rep, NeedReboot: true}, nil
}

func (c *Checker) save(rep api.HealthReport, reportOnly bool) error {
	path := c.Cfg.HealthReportPath()
	if reportOnly {
		path = filepath.Join(c.Cfg.Paths.RunDir, "health-periodic.json")
	}
	return config.WriteJSONAtomic(path, rep, 0o644)
}

func result(cs []api.HealthCheck) string {
	if AllOK(cs) {
		return "pass"
	}
	return "fail"
}

func failedNames(cs []api.HealthCheck) string {
	var n []string
	for _, c := range cs {
		if !c.OK {
			n = append(n, c.Name+": "+c.Detail)
		}
	}
	return strings.Join(n, "; ")
}

// NetworkDown reports whether the network is down for reasons outside the
// device: no carrier on any interface, or no default route (no DHCP lease).
func NetworkDown(root string) (bool, string) {
	ifaces, _ := filepath.Glob(filepath.Join(root, "sys/class/net/*"))
	var up []string
	for _, i := range ifaces {
		name := filepath.Base(i)
		if name == "lo" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(i, "carrier"))
		if err == nil && strings.TrimSpace(string(b)) == "1" {
			up = append(up, name)
		}
	}
	if len(up) == 0 {
		return true, "no link"
	}
	if !hasDefaultRoute(root) {
		return true, "no default route (links up: " + strings.Join(up, ",") + ")"
	}
	return false, "links up: " + strings.Join(up, ",")
}

func hasDefaultRoute(root string) bool {
	if f, err := os.Open(filepath.Join(root, "proc/net/route")); err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fs := strings.Fields(sc.Text())
			if len(fs) > 2 && fs[1] == "00000000" && fs[0] != "Iface" {
				return true
			}
		}
	}
	if f, err := os.Open(filepath.Join(root, "proc/net/ipv6_route")); err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fs := strings.Fields(sc.Text())
			if len(fs) == 10 && fs[0] == strings.Repeat("0", 32) && fs[1] == "00" && fs[9] != "lo" {
				return true
			}
		}
	}
	return false
}

func isMountPoint(root, path string) bool {
	f, err := os.Open(filepath.Join(root, "proc/self/mountinfo"))
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) > 4 && fs[4] == path {
			return true
		}
	}
	return false
}

func orNone(s []string) string {
	if len(s) == 0 {
		return "none"
	}
	return strings.Join(s, ", ")
}

func errOrOK(err error) any {
	if err == nil {
		return "ok"
	}
	return err
}
