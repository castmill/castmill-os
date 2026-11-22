package health

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/castmill/castmill-os/agent/internal/api"
	"github.com/castmill/castmill-os/agent/internal/config"
	"github.com/castmill/castmill-os/agent/internal/rauc"
	"github.com/castmill/castmill-os/agent/internal/state"
)

type fakeRAUC struct {
	status *rauc.Status
	marks  []string
}

func (f *fakeRAUC) Status(context.Context) (*rauc.Status, error) { return f.status, nil }
func (f *fakeRAUC) Install(context.Context, string, rauc.InstallOptions, func(rauc.Progress)) error {
	return nil
}
func (f *fakeRAUC) Mark(_ context.Context, st, slot string) error {
	f.marks = append(f.marks, st+":"+slot)
	return nil
}

type fakeSystemd struct{ states map[string]string }

func (f *fakeSystemd) RestartUnit(context.Context, string) error { return nil }
func (f *fakeSystemd) Reboot(context.Context) error              { return nil }
func (f *fakeSystemd) UnitActive(_ context.Context, u string) (string, error) {
	if s, ok := f.states[u]; ok {
		return s, nil
	}
	return "active", nil
}
func (f *fakeSystemd) FailedUnits(context.Context) ([]string, error) { return nil, nil }

type env struct {
	t     *testing.T
	root  string
	cfg   *config.Agent
	rauc  *fakeRAUC
	sd    *fakeSystemd
	now   time.Time
	check *Checker
}

func write(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newEnv(t *testing.T) *env {
	root := t.TempDir()
	cfg := config.Defaults()
	cfg.Paths.DataRoot = filepath.Join(root, "data")
	cfg.Paths.DataDir = filepath.Join(root, "data/castmill")
	cfg.Paths.RunDir = filepath.Join(root, "run")
	cfg.Paths.LogDir = filepath.Join(root, "data/log")
	cfg.Health.Timeout = config.Duration{Duration: 0} // single attempt
	os.MkdirAll(cfg.Paths.DataRoot, 0o755)
	write(t, filepath.Join(root, "proc/self/mountinfo"), "30 1 254:4 / "+cfg.Paths.DataRoot+" rw,noatime - ext4 /dev/vda4 rw\n")
	write(t, filepath.Join(root, "sys/class/drm/card0-Virtual-1/status"), "connected\n")
	write(t, filepath.Join(root, "sys/class/drm/card0-Virtual-2/status"), "disconnected\n")
	write(t, filepath.Join(root, "sys/class/net/eth0/carrier"), "1\n")
	write(t, filepath.Join(root, "proc/net/route"), "Iface\tDestination\tGateway\neth0\t00000000\t0202000A\n")

	e := &env{t: t, root: root, cfg: cfg, now: time.Date(2026, 11, 22, 3, 30, 0, 0, time.UTC)}
	e.rauc = &fakeRAUC{status: &rauc.Status{BootSlot: "B", Slots: []rauc.Slot{
		{Name: "rootfs.0", Class: "rootfs", Bootname: "A", State: "inactive", BootStatus: "good", BundleVersion: "2026.11.0"},
		{Name: "rootfs.1", Class: "rootfs", Bootname: "B", State: "booted", BootStatus: "good", BundleVersion: "2026.12.0"},
	}}}
	e.sd = &fakeSystemd{states: map[string]string{}}
	e.check = &Checker{Cfg: cfg, RAUC: e.rauc, Systemd: e.sd, SysRoot: root,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return e.now }, Interval: time.Millisecond}
	e.runtime(func(r *state.Runtime) {})
	return e
}

// runtime writes a healthy agent state, then applies mod.
func (e *env) runtime(mod func(r *state.Runtime)) {
	r := &state.Runtime{Updated: e.now, BackendOK: e.now.Add(-time.Minute),
		Outputs: map[string]state.OutputState{"Virtual-1": {Connected: true, LastFrame: e.now.Add(-2 * time.Second)}}}
	mod(r)
	if err := config.WriteJSONAtomic(e.cfg.RuntimeStatePath(), r, 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) trial(target string) {
	if err := state.SaveUpdate(filepath.Join(e.cfg.StateDir(), "update.json"),
		state.Update{State: api.UpdateActivating, Version: "2026.12.0", TargetSlot: target}); err != nil {
		e.t.Fatal(err)
	}
}

func TestTrialPassCommits(t *testing.T) {
	e := newEnv(t)
	e.trial("B")
	out, err := e.check.Run(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if out.NeedReboot || !out.Report.Committed || out.Report.Result != "pass" || !out.Report.Trial {
		t.Fatalf("unexpected outcome: %+v", out)
	}
	if len(e.rauc.marks) != 1 || e.rauc.marks[0] != "good:booted" {
		t.Fatalf("marks = %v", e.rauc.marks)
	}
	var saved api.HealthReport
	if err := config.ReadJSON(e.cfg.HealthReportPath(), &saved); err != nil || !saved.Committed {
		t.Fatalf("report not saved: %v %+v", err, saved)
	}
}

func TestTrialFailRejects(t *testing.T) {
	e := newEnv(t)
	e.trial("B")
	e.runtime(func(r *state.Runtime) { r.Outputs = nil }) // player never rendered
	out, err := e.check.Run(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !out.NeedReboot || out.Report.Result != "fail" || out.Report.Committed {
		t.Fatalf("unexpected outcome: %+v", out.Report)
	}
	if len(e.rauc.marks) != 1 || e.rauc.marks[0] != "bad:booted" {
		t.Fatalf("marks = %v", e.rauc.marks)
	}
}

func TestTrialBackendFaultRejects(t *testing.T) {
	e := newEnv(t)
	e.trial("B")
	e.runtime(func(r *state.Runtime) { r.BackendOK = time.Time{}; r.BackendError = "tls: bad certificate" })
	out, _ := e.check.Run(context.Background(), false)
	if !out.NeedReboot {
		t.Fatalf("network up but backend unreachable must fail the trial: %+v", out.Report.Checks)
	}
}

func TestTrialNetworkOutageDoesNotRollBack(t *testing.T) {
	e := newEnv(t)
	e.trial("B")
	e.runtime(func(r *state.Runtime) { r.BackendOK = time.Time{} })
	write(t, filepath.Join(e.root, "sys/class/net/eth0/carrier"), "0\n") // cable unplugged
	out, _ := e.check.Run(context.Background(), false)
	if out.NeedReboot || !out.Report.Committed {
		t.Fatalf("a network outage must not cause a rollback: %+v", out.Report.Checks)
	}
	e = newEnv(t)
	e.trial("B")
	e.runtime(func(r *state.Runtime) { r.BackendOK = time.Time{} })
	write(t, filepath.Join(e.root, "proc/net/route"), "Iface\tDestination\tGateway\n") // no DHCP lease
	out, _ = e.check.Run(context.Background(), false)
	if out.NeedReboot {
		t.Fatalf("no default route counts as external outage: %+v", out.Report.Checks)
	}
}

func TestCommittedSlotNeverRejected(t *testing.T) {
	e := newEnv(t)
	e.runtime(func(r *state.Runtime) { r.Outputs = nil })
	e.sd.states["castmill-session.service"] = "failed"
	out, err := e.check.Run(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if out.NeedReboot || out.Report.Trial || out.Report.Result != "fail" {
		t.Fatalf("unexpected: %+v", out.Report)
	}
	if len(e.rauc.marks) != 1 || e.rauc.marks[0] != "good:booted" {
		t.Fatalf("normal boot must only mark-good: %v", e.rauc.marks)
	}
}

func TestFallbackAfterFailedBootMarksOtherBad(t *testing.T) {
	e := newEnv(t)
	e.trial("A") // A was activated, but the bootloader fell back to B
	out, _ := e.check.Run(context.Background(), false)
	if out.Report.Trial || out.NeedReboot {
		t.Fatalf("unexpected: %+v", out.Report)
	}
	if len(e.rauc.marks) != 2 || e.rauc.marks[1] != "bad:other" {
		t.Fatalf("marks = %v", e.rauc.marks)
	}
}

func TestReportOnlyNeverMarks(t *testing.T) {
	e := newEnv(t)
	e.trial("B")
	if _, err := e.check.Run(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(e.rauc.marks) != 0 {
		t.Fatalf("report-only marked slots: %v", e.rauc.marks)
	}
}

func TestNoRAUCIsReportOnly(t *testing.T) {
	e := newEnv(t)
	e.check.RAUC = nil
	out, err := e.check.Run(context.Background(), false)
	if err != nil || out.NeedReboot || out.Report.Committed || out.Report.Result != "pass" {
		t.Fatalf("unexpected: %v %+v", err, out)
	}
}
