package agent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/castmill/castmill-os/agent/internal/config"
	"github.com/castmill/castmill-os/agent/internal/state"
)

func (p *fakePlayer) callArgs(method string, args map[string]any) map[string]any {
	p.t.Helper()
	p.mu.Lock()
	p.seq++
	id := fmt.Sprint(p.seq)
	p.mu.Unlock()
	p.send(map[string]any{"type": "call", "id": id, "method": method, "args": args})
	var r map[string]any
	eventually(p.t, "reply to "+method, 5*time.Second, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		r = p.replies[id]
		return r != nil
	})
	return r
}

func TestDeviceSchedule(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer backend.Close()
	cfg, sys := setup(t, backend.URL)
	// The schedule only runs on a trusted clock.
	if err := os.WriteFile(filepath.Join(cfg.Paths.RunDir, "time-from-backend"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	start(t, &Agent{Cfg: cfg, SysRoot: sys})
	p := dialPlayer(t, cfg.PlayerSocketPath(), "Virtual-1")

	display := func() (state.DisplayState, bool) {
		rt, err := state.LoadRuntime(cfg.RuntimeStatePath())
		if err != nil {
			return state.DisplayState{}, false
		}
		d, ok := rt.Display["Virtual-1"]
		return d, ok
	}

	// Off every day at midnight, never on: the display is off now. The test
	// system has no CEC or DDC/CI, so only the state changes.
	off := `{"on":[],"off":[{"hours":0,"minutes":0,"weekDays":["ALL"]}]}`
	if r := p.callArgs("set_timers", map[string]any{"timers": off}); r["ok"] != true {
		t.Fatalf("set_timers: %v", r)
	}
	eventually(t, "display off", 10*time.Second, func() bool {
		d, ok := display()
		return ok && d.Power == state.PowerOff && d.Method == "none" && d.Error != ""
	})
	var saved displaySchedules
	if err := config.ReadJSON(filepath.Join(cfg.StateDir(), "display-schedule.json"), &saved); err != nil ||
		len(saved.Outputs["Virtual-1"].Off) != 1 {
		t.Fatalf("schedule not saved: %+v, %v", saved, err)
	}

	// Bad input is rejected.
	if r := p.callArgs("set_timers", map[string]any{"timers": `{"on":[{"hours":25}]}`}); r["ok"] == true {
		t.Fatal("invalid timers accepted")
	}

	// Removing the schedule switches the display back on, then leaves it alone.
	if r := p.callArgs("set_timers", map[string]any{"timers": ""}); r["ok"] != true {
		t.Fatalf("set_timers: %v", r)
	}
	eventually(t, "schedule removed", 10*time.Second, func() bool {
		d, ok := display()
		return !ok || d.Power == state.PowerOn
	})
}
