package updater

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/castmill/castmill-os/agent/internal/api"
	"github.com/castmill/castmill-os/agent/internal/config"
	"github.com/castmill/castmill-os/agent/internal/maintenance"
	"github.com/castmill/castmill-os/agent/internal/rauc"
	"github.com/castmill/castmill-os/agent/internal/state"
)

// fakeDevice simulates RAUC + bootloader for two slots.
type fakeDevice struct {
	booted     string            // bootname
	versions   map[string]string // bootname -> bundle version
	primary    string            // slot the bootloader tries next
	installErr error
	installs   int
	reboots    int
	marks      []string
}

func (f *fakeDevice) other() string {
	if f.booted == "A" {
		return "B"
	}
	return "A"
}

func (f *fakeDevice) Status(context.Context) (*rauc.Status, error) {
	st := &rauc.Status{BootSlot: f.booted}
	for i, b := range []string{"A", "B"} {
		s := rauc.Slot{Name: "rootfs." + string(rune('0'+i)), Class: "rootfs", Bootname: b, State: "inactive", BundleVersion: f.versions[b]}
		if b == f.booted {
			s.State = "booted"
		}
		st.Slots = append(st.Slots, s)
	}
	return st, nil
}

func (f *fakeDevice) Install(_ context.Context, src string, _ rauc.InstallOptions, p func(rauc.Progress)) error {
	f.installs++
	if f.installErr != nil {
		return f.installErr
	}
	if p != nil {
		p(rauc.Progress{Percent: 50})
	}
	f.versions[f.other()] = "v-from-" + src
	return nil
}

func (f *fakeDevice) Mark(_ context.Context, st, slot string) error {
	f.marks = append(f.marks, st+":"+slot)
	if st == "active" && slot == "other" {
		f.primary = f.other()
	}
	return nil
}

func (f *fakeDevice) Reboot(context.Context) error { f.reboots++; return nil }

type harness struct {
	t   *testing.T
	dir string
	dev *fakeDevice
	now time.Time
	pol maintenance.Policy
}

func newHarness(t *testing.T) *harness {
	return &harness{t: t, dir: t.TempDir(),
		dev: &fakeDevice{booted: "A", primary: "A", versions: map[string]string{"A": "v-old"}},
		now: time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC), // noon: outside the default window
		pol: maintenance.Policy{Location: time.UTC, TimeTrusted: true}}
}

// boot creates a fresh Updater, as after a (re)boot of the agent.
func (h *harness) boot() *Updater {
	u := &Updater{RAUC: h.dev, Rebooter: h.dev, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		StatePath: filepath.Join(h.dir, "update.json"), HealthPath: filepath.Join(h.dir, "health.json"),
		Policy: func() maintenance.Policy { return h.pol }, Now: func() time.Time { return h.now }}
	if err := u.Resume(context.Background()); err != nil {
		h.t.Fatal(err)
	}
	return u
}

func (h *harness) health(slot string, committed bool, result string) {
	if err := config.WriteJSONAtomic(filepath.Join(h.dir, "health.json"), api.HealthReport{Slot: slot,
		Committed: committed, Result: result, Checks: []api.HealthCheck{{Name: "frames", OK: result == "pass", Detail: "x"}}}, 0o644); err != nil {
		h.t.Fatal(err)
	}
}

func step(t *testing.T, u *Updater) {
	t.Helper()
	if err := u.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHappyPath(t *testing.T) {
	h := newHarness(t)
	u := h.boot()
	u.SetDesired(&api.Desired{Version: "v-from-url1", BundleURL: "url1"})
	step(t, u)
	if s := u.Status(); s.State != api.UpdateInstalled || s.Progress != 100 {
		t.Fatalf("after install: %+v", s)
	}
	step(t, u) // noon: outside window
	if h.dev.reboots != 0 {
		t.Fatal("rebooted outside the maintenance window")
	}

	// Power cut before the window: the bootloader must still boot the old slot.
	h.dev.booted = h.dev.primary
	u = h.boot()
	if h.dev.booted != "A" || u.Status().State != api.UpdateInstalled {
		t.Fatalf("power cut activated the update: booted=%s state=%s", h.dev.booted, u.Status().State)
	}

	h.now = time.Date(2026, 11, 24, 3, 15, 0, 0, time.UTC)
	step(t, u)
	if h.dev.reboots != 1 || u.Status().State != api.UpdateActivating || h.dev.primary != "B" {
		t.Fatalf("activation: reboots=%d state=%s primary=%s", h.dev.reboots, u.Status().State, h.dev.primary)
	}

	// Reboot into B; health still pending.
	h.dev.booted = "B"
	u = h.boot()
	if s := u.Status().State; s != api.UpdateActivating {
		t.Fatalf("before commit: %s", s)
	}
	h.health("B", true, "pass")
	u.RefreshCommit("B")
	if s := u.Status().State; s != api.UpdateRunning {
		t.Fatalf("after commit: %s", s)
	}
	u.MarkReported()
	if s := u.Status().State; s != api.UpdateIdle {
		t.Fatalf("after report: %s", s)
	}
	step(t, u) // desired == booted: nothing to do
	if h.dev.installs != 1 {
		t.Fatalf("reinstalled the running version")
	}
}

func TestRollbackNotRetried(t *testing.T) {
	h := newHarness(t)
	h.pol.Windows = []api.Window{{Start: "00:00", End: "00:00"}} // always
	u := h.boot()
	u.SetDesired(&api.Desired{Version: "v-from-bad", BundleURL: "bad"})
	step(t, u)
	step(t, u)
	if u.Status().State != api.UpdateActivating {
		t.Fatalf("state %s", u.Status().State)
	}
	// B boots, health fails -> mark-bad -> bootloader falls back to A.
	h.health("B", false, "fail")
	h.dev.booted = "A"
	u = h.boot()
	s := u.Status()
	if s.State != api.UpdateRolledBack || s.LastError == "" {
		t.Fatalf("expected rolled_back with reason, got %+v", s)
	}
	u.SetDesired(&api.Desired{Version: "v-from-bad", BundleURL: "bad"})
	step(t, u)
	if h.dev.installs != 1 {
		t.Fatal("a rolled-back version was installed again")
	}
	// A new version is accepted.
	u.SetDesired(&api.Desired{Version: "v-from-good", BundleURL: "good"})
	step(t, u)
	if h.dev.installs != 2 || u.Status().State != api.UpdateInstalled {
		t.Fatalf("new version not installed: %+v", u.Status())
	}
}

func TestInterruptedInstallRestarts(t *testing.T) {
	h := newHarness(t)
	u := h.boot()
	u.set(func(s *state.Update) { s.State = api.UpdateInstalling; s.Version = "v-x" })
	u = h.boot()
	if s := u.Status(); s.State != api.UpdateIdle {
		t.Fatalf("interrupted install should reset to idle: %+v", s)
	}
}

func TestInstallFailureBacksOff(t *testing.T) {
	h := newHarness(t)
	h.dev.installErr = errors.New("signature verification failed")
	u := h.boot()
	u.SetDesired(&api.Desired{Version: "v1", BundleURL: "u"})
	if err := u.Step(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if s := u.Status(); s.State != api.UpdateFailed || s.LastError == "" {
		t.Fatalf("%+v", s)
	}
	_ = u.Step(context.Background())
	if h.dev.installs != 1 {
		t.Fatal("retried without back-off")
	}
	h.now = h.now.Add(2 * time.Minute)
	_ = u.Step(context.Background())
	if h.dev.installs != 2 {
		t.Fatal("did not retry after back-off")
	}
}

func TestPinnedAndApplyNow(t *testing.T) {
	h := newHarness(t)
	u := h.boot()
	u.SetDesired(&api.Desired{Version: "v1", BundleURL: "u", Pinned: true})
	step(t, u)
	if h.dev.installs != 0 {
		t.Fatal("installed while pinned")
	}
	u.SetDesired(&api.Desired{Version: "v-from-u", BundleURL: "u"})
	step(t, u)
	if err := u.ForceActivate(); err != nil {
		t.Fatal(err)
	}
	h.pol.TimeTrusted = false
	step(t, u)
	if h.dev.reboots != 1 {
		t.Fatal("apply-now did not reboot")
	}
}
