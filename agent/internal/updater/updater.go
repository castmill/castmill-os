// Package updater orchestrates OS updates on the device (spec 03).
//
//	idle --(desired != booted)--> installing --ok--> installed
//	installed --(maintenance window / apply now)--> activating --reboot-->
//	   booted target, health commits  --> running (reported) --> idle
//	   bootloader fallback / mark-bad  --> rolled_back (not retried for that version)
//
// RAUC is configured with activate-installed=false: installing never changes
// the boot order, so an unexpected reboot (power cut) can't activate an update
// outside the maintenance window. Activation is an explicit mark-active.
package updater

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/castmill/castmill-os/agent/internal/api"
	"github.com/castmill/castmill-os/agent/internal/config"
	"github.com/castmill/castmill-os/agent/internal/maintenance"
	"github.com/castmill/castmill-os/agent/internal/rauc"
	"github.com/castmill/castmill-os/agent/internal/state"
)

// Rebooter reboots the device.
type Rebooter interface {
	Reboot(ctx context.Context) error
}

// Updater owns the update state. Methods are safe for concurrent use; Step
// must be called from a single goroutine.
type Updater struct {
	RAUC       rauc.Client
	Rebooter   Rebooter
	StatePath  string // /data/castmill/state/update.json
	HealthPath string // /data/log/castmill/health.json
	Log        *slog.Logger
	// Policy returns the current activation policy inputs (windows, tz, time
	// trust); ActivatePolicy/InstalledAt are filled in by the updater.
	Policy      func() maintenance.Policy
	InstallOpts func() rauc.InstallOptions
	// OnChange is called after every state change (to report to the backend).
	OnChange func(api.UpdateStatus)
	Now      func() time.Time

	mu       sync.Mutex
	st       state.Update
	desired  *api.Desired
	progress int
	// failed remembers versions that rolled back, so they aren't retried.
	failed map[string]bool
	// Back-off after failed installs.
	nextAttempt time.Time
	backoff     time.Duration
}

func (u *Updater) now() time.Time {
	if u.Now != nil {
		return u.Now()
	}
	return time.Now()
}

// Resume loads the persisted state after a (re)boot and resolves the outcome
// of an activation. Call once at start-up.
func (u *Updater) Resume(ctx context.Context) error {
	u.mu.Lock()
	u.st = state.LoadUpdate(u.StatePath)
	u.failed = map[string]bool{}
	if u.st.State == api.UpdateRolledBack {
		u.failed[u.st.Version] = true
	}
	u.mu.Unlock()

	st, err := u.RAUC.Status(ctx)
	if err != nil {
		return err
	}
	booted := st.Booted()
	if booted == nil {
		return errors.New("no booted slot")
	}
	switch u.get().State {
	case api.UpdateInstalling:
		// Interrupted (power cut / agent restart): the inactive slot is
		// incomplete. Start again from idle.
		u.set(func(s *state.Update) { s.State, s.LastError = api.UpdateIdle, "installation interrupted" })
	case api.UpdateInstalled:
		if o := st.Other(); o == nil || o.BundleVersion != u.get().Version {
			u.set(func(s *state.Update) { s.State = api.UpdateIdle })
		}
	case api.UpdateActivating:
		cur := u.get()
		if booted.Bootname == cur.TargetSlot {
			var rep api.HealthReport
			if err := config.ReadJSON(u.HealthPath, &rep); err == nil && rep.Committed && rep.Slot == booted.Bootname {
				u.set(func(s *state.Update) { s.State, s.LastError, s.Reported = api.UpdateRunning, "", false })
			}
			// Otherwise the health check is still running; WaitCommit picks it up.
		} else {
			reason := "new version failed to boot (bootloader fallback)"
			var rep api.HealthReport
			if err := config.ReadJSON(u.HealthPath, &rep); err == nil && rep.Slot == cur.TargetSlot && rep.Result == "fail" {
				reason = "health check failed: " + summarize(rep.Checks)
			}
			u.mu.Lock()
			u.failed[cur.Version] = true
			u.mu.Unlock()
			u.set(func(s *state.Update) { s.State, s.LastError, s.Reported = api.UpdateRolledBack, reason, false })
			u.Log.Error("update rolled back", "version", cur.Version, "reason", reason)
		}
	}
	return nil
}

// RefreshCommit checks whether a pending trial boot has been committed by
// castmill-health since Resume.
func (u *Updater) RefreshCommit(bootedSlot string) {
	cur := u.get()
	if cur.State != api.UpdateActivating || cur.TargetSlot != bootedSlot {
		return
	}
	var rep api.HealthReport
	if err := config.ReadJSON(u.HealthPath, &rep); err == nil && rep.Committed && rep.Slot == bootedSlot {
		u.set(func(s *state.Update) { s.State, s.LastError, s.Reported = api.UpdateRunning, "", false })
	}
}

// SetDesired records the backend's desired version.
func (u *Updater) SetDesired(d *api.Desired) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.desired = d
}

// ForceActivate marks the installed update for immediate activation.
func (u *Updater) ForceActivate() error {
	if u.get().State != api.UpdateInstalled {
		return errors.New("no installed update waiting for activation")
	}
	u.set(func(s *state.Update) { s.ActivatePolicy = api.ActivateNow })
	return nil
}

// Status is the wire representation.
func (u *Updater) Status() api.UpdateStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	return api.UpdateStatus{State: u.st.State, Version: u.st.Version, Progress: u.progress,
		LastError: u.st.LastError, Updated: u.now().UTC()}
}

// MarkReported acknowledges that a final outcome reached the backend; the
// state machine then returns to idle.
func (u *Updater) MarkReported() {
	cur := u.get()
	if cur.State == api.UpdateRunning {
		u.set(func(s *state.Update) { s.Reported = true; s.State = api.UpdateIdle })
	} else if cur.State == api.UpdateRolledBack && !cur.Reported {
		u.set(func(s *state.Update) { s.Reported = true })
	}
}

func (u *Updater) get() state.Update {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.st
}

func (u *Updater) set(f func(*state.Update)) {
	u.mu.Lock()
	f(&u.st)
	s := u.st
	u.mu.Unlock()
	if err := state.SaveUpdate(u.StatePath, s); err != nil {
		u.Log.Error("saving update state", "err", err)
	}
	if u.OnChange != nil {
		u.OnChange(u.Status())
	}
}

// Step performs at most one action (install or activate). It blocks while
// installing.
func (u *Updater) Step(ctx context.Context) error {
	u.mu.Lock()
	d := u.desired
	u.mu.Unlock()
	cur := u.get()

	switch cur.State {
	case api.UpdateActivating, api.UpdateRunning:
		return nil // waiting for the reboot / health check / report
	case api.UpdateInstalled:
		if d != nil && d.Version != "" && d.Version != cur.Version {
			// Superseded before activation: install the newer one.
			u.set(func(s *state.Update) { s.State = api.UpdateIdle })
			return u.Step(ctx)
		}
		return u.maybeActivate(ctx)
	}

	if d == nil || d.Version == "" || d.Pinned {
		return nil
	}
	st, err := u.RAUC.Status(ctx)
	if err != nil {
		return err
	}
	booted, other := st.Booted(), st.Other()
	if booted == nil || other == nil {
		return errors.New("A/B slots not found")
	}
	if d.Version == booted.BundleVersion {
		return nil
	}
	u.mu.Lock()
	skip := u.failed[d.Version]
	u.mu.Unlock()
	if skip {
		return nil // rolled back before; the backend must offer a different version
	}
	if d.BundleURL == "" {
		return errors.New("desired version without bundle URL")
	}
	u.mu.Lock()
	wait := u.now().Before(u.nextAttempt)
	u.mu.Unlock()
	if wait {
		return nil
	}
	err = u.install(ctx, d, booted.BundleVersion, other.Bootname)
	u.mu.Lock()
	if err != nil {
		u.backoff = min(max(2*u.backoff, time.Minute), time.Hour)
		u.nextAttempt = u.now().Add(u.backoff)
	} else {
		u.backoff, u.nextAttempt = 0, time.Time{}
	}
	u.mu.Unlock()
	return err
}

func (u *Updater) install(ctx context.Context, d *api.Desired, from, target string) error {
	u.Log.Info("installing update", "version", d.Version, "url", d.BundleURL, "slot", target)
	u.mu.Lock()
	u.progress = 0
	u.mu.Unlock()
	u.set(func(s *state.Update) {
		*s = state.Update{State: api.UpdateInstalling, Version: d.Version, BundleURL: d.BundleURL,
			ActivatePolicy: d.ActivatePolicy, Security: d.Security, TargetSlot: target, FromVersion: from}
	})
	var opts rauc.InstallOptions
	if u.InstallOpts != nil {
		opts = u.InstallOpts()
	}
	lastReport := time.Time{}
	err := u.RAUC.Install(ctx, d.BundleURL, opts, func(p rauc.Progress) {
		u.mu.Lock()
		u.progress = p.Percent
		u.mu.Unlock()
		if u.OnChange != nil && time.Since(lastReport) > 10*time.Second {
			lastReport = time.Now()
			u.OnChange(u.Status())
		}
	})
	if err != nil {
		u.set(func(s *state.Update) { s.State, s.LastError = api.UpdateFailed, err.Error() })
		return fmt.Errorf("install %s: %w", d.Version, err)
	}
	u.mu.Lock()
	u.progress = 100
	u.mu.Unlock()
	u.set(func(s *state.Update) { s.State, s.InstalledAt, s.LastError = api.UpdateInstalled, u.now().UTC(), "" })
	u.Log.Info("update installed; waiting for activation", "version", d.Version)
	return nil
}

func (u *Updater) maybeActivate(ctx context.Context) error {
	cur := u.get()
	p := maintenance.Policy{}
	if u.Policy != nil {
		p = u.Policy()
	}
	p.ActivatePolicy, p.InstalledAt = cur.ActivatePolicy, cur.InstalledAt
	dec := maintenance.ShouldActivate(p, u.now())
	if !dec.Activate {
		return nil
	}
	if u.Rebooter == nil {
		return errors.New("cannot activate: reboot not available")
	}
	u.Log.Info("activating update", "version", cur.Version, "reason", dec.Reason)
	if err := u.RAUC.Mark(ctx, "active", "other"); err != nil {
		u.set(func(s *state.Update) { s.LastError = err.Error() })
		return err
	}
	u.set(func(s *state.Update) { s.State, s.ActivatedAt = api.UpdateActivating, u.now().UTC() })
	return u.Rebooter.Reboot(ctx)
}

func summarize(cs []api.HealthCheck) string {
	out := ""
	for _, c := range cs {
		if !c.OK {
			if out != "" {
				out += "; "
			}
			out += c.Name + ": " + c.Detail
		}
	}
	return out
}
