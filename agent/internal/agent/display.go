package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/castmill/castmill-os/agent/internal/config"
	"github.com/castmill/castmill-os/agent/internal/display"
	"github.com/castmill/castmill-os/agent/internal/state"
	"github.com/castmill/castmill-os/agent/internal/system"
)

// Device schedule (MED-006). Castmill's device app stores its timers through
// the castmillOS bridge and blanks the screen on schedule; the kiosk shell
// forwards them here, and the agent puts the display itself in standby.
// Schedules are kept on /data: a TV in standby may disconnect from HDMI,
// which stops its player, and the agent must still wake it.

const (
	displayTick       = 30 * time.Second
	displayRetry      = 2 * time.Minute
	displayRetryNoCtl = 10 * time.Minute // no CEC/DDC: a display may be plugged in later
)

type displaySchedules struct {
	Outputs map[string]display.Timers `json:"outputs"`
}

func (a *Agent) displaySchedulePath() string {
	return filepath.Join(a.Cfg.StateDir(), "display-schedule.json")
}

func (a *Agent) loadDisplaySchedules() {
	var s displaySchedules
	_ = config.ReadJSON(a.displaySchedulePath(), &s)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.schedules = s.Outputs
	if a.schedules == nil {
		a.schedules = map[string]display.Timers{}
	}
	a.displayPower = map[string]*state.DisplayState{}
}

// setTimers handles the "set_timers" call: {"timers": "<TIMERS setting>"}.
func (a *Agent) setTimers(output string, args json.RawMessage) error {
	var req struct {
		Timers string `json:"timers"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return fmt.Errorf("set_timers: %w", err)
	}
	if !display.ValidOutput(output) {
		return fmt.Errorf("set_timers: invalid output %q", output)
	}
	t, err := display.ParseTimers(req.Timers)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.schedules[output] = t
	s := displaySchedules{Outputs: map[string]display.Timers{}}
	for o, ot := range a.schedules {
		if !ot.Empty() {
			s.Outputs[o] = ot
		}
	}
	a.mu.Unlock()
	if err := config.WriteJSONAtomic(a.displaySchedulePath(), s, 0o644); err != nil {
		return err
	}
	a.Log.Info("device schedule", "output", output, "on", len(t.On), "off", len(t.Off))
	select {
	case a.displayKick <- struct{}{}:
	default:
	}
	return nil
}

func (a *Agent) displayLoop(ctx context.Context) {
	t := time.NewTicker(displayTick)
	defer t.Stop()
	for {
		a.applySchedules(time.Now())
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-a.displayKick:
		}
	}
}

// applySchedules switches each scheduled display to the state its schedule
// wants, once per change: a display someone switches on by hand stays on
// until the next scheduled change.
func (a *Agent) applySchedules(now time.Time) {
	if !system.TimeTrusted(a.Cfg.Paths.RunDir) {
		return // a wrong clock would switch screens at the wrong times
	}
	loc := time.Local
	if l, err := time.LoadLocation(a.timezone()); err == nil {
		loc = l
	}
	now = now.In(loc)

	a.mu.Lock()
	type job struct {
		output string
		on     bool
	}
	var jobs []job
	for output, t := range a.schedules {
		p := a.displayPower[output]
		if p == nil {
			p = &state.DisplayState{}
			a.displayPower[output] = p
		}
		want := state.PowerOn
		if t.ShouldBeOff(now) {
			want = state.PowerOff
		}
		if t.Empty() && p.Power != state.PowerOff {
			delete(a.schedules, output) // no schedule, and we didn't switch it off
			delete(a.displayPower, output)
			continue
		}
		if p.Power == want && (p.Error == "" || now.Before(p.RetryAt)) {
			continue
		}
		if p.Power != want {
			p.Since = now.UTC()
		}
		p.Power = want
		jobs = append(jobs, job{output, want == state.PowerOn})
	}
	a.mu.Unlock()

	for _, j := range jobs {
		method, err := a.display.SetPower(j.output, j.on)
		a.mu.Lock()
		p := a.displayPower[j.output]
		if p == nil {
			a.mu.Unlock()
			continue
		}
		prevErr := p.Error
		p.Method, p.Error, p.RetryAt = method, "", time.Time{}
		switch {
		case errors.Is(err, display.ErrUnsupported):
			p.Method, p.Error, p.RetryAt = "none", err.Error(), now.Add(displayRetryNoCtl)
		case err != nil:
			p.Error, p.RetryAt = err.Error(), now.Add(displayRetry)
		}
		st := *p
		a.mu.Unlock()
		switch {
		case st.Error == "":
			a.Log.Info("display switched by schedule", "output", j.output, "power", st.Power, "method", st.Method)
		case st.Error != prevErr:
			// The device app still blanks the screen; only standby is missing.
			a.Log.Warn("display not switched by schedule", "output", j.output, "power", st.Power, "err", st.Error)
		}
	}
}
