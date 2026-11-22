package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/castmill/castmill-os/agent/internal/api"
	"github.com/castmill/castmill-os/agent/internal/config"
	"github.com/castmill/castmill-os/agent/internal/rauc"
	"github.com/castmill/castmill-os/agent/internal/state"
)

// fakePlayer stands in for one kiosk-shell process.
type fakePlayer struct {
	t       *testing.T
	conn    net.Conn
	mu      sync.Mutex
	states  []map[string]any
	replies map[string]map[string]any
	reloads int
	seq     int
}

func dialPlayer(t *testing.T, sock, output string) *fakePlayer {
	t.Helper()
	var c net.Conn
	var err error
	for i := 0; i < 100; i++ {
		if c, err = net.Dial("unix", sock); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	p := &fakePlayer{t: t, conn: c, replies: map[string]map[string]any{}}
	p.send(map[string]any{"type": "hello", "output": output})
	go p.read()
	return p
}

func (p *fakePlayer) send(m map[string]any) {
	b, _ := json.Marshal(m)
	if _, err := p.conn.Write(append(b, '\n')); err != nil {
		p.t.Log("player write:", err)
	}
}

func (p *fakePlayer) read() {
	sc := bufio.NewScanner(p.conn)
	for sc.Scan() {
		var m map[string]any
		_ = json.Unmarshal(sc.Bytes(), &m)
		p.mu.Lock()
		switch m["type"] {
		case "state":
			p.states = append(p.states, m["state"].(map[string]any))
		case "reply":
			p.replies[m["id"].(string)] = m
		case "reload":
			p.reloads++
		}
		p.mu.Unlock()
	}
}

func (p *fakePlayer) lastState() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.states) == 0 {
		return nil
	}
	return p.states[len(p.states)-1]
}

// call performs a bridge call and waits for the reply.
func (p *fakePlayer) call(method string) map[string]any {
	p.t.Helper()
	p.mu.Lock()
	p.seq++
	id := fmt.Sprint(p.seq)
	p.mu.Unlock()
	p.send(map[string]any{"type": "call", "id": id, "method": method})
	var r map[string]any
	eventually(p.t, "reply to "+method, 5*time.Second, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		r = p.replies[id]
		return r != nil
	})
	return r
}

func eventually(t *testing.T, what string, timeout time.Duration, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// fakeRAUC records installs.
type fakeRAUC struct {
	mu       sync.Mutex
	installs []string
}

func (f *fakeRAUC) Status(context.Context) (*rauc.Status, error) {
	return &rauc.Status{BootSlot: "A", Slots: []rauc.Slot{
		{Name: "rootfs.0", Class: "rootfs", Bootname: "A", State: "booted"},
		{Name: "rootfs.1", Class: "rootfs", Bootname: "B", State: "inactive"},
	}}, nil
}
func (f *fakeRAUC) Install(_ context.Context, src string, _ rauc.InstallOptions, _ func(rauc.Progress)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installs = append(f.installs, src)
	return nil
}
func (f *fakeRAUC) Mark(context.Context, string, string) error { return nil }

func setup(t *testing.T, backendURL string) (*config.Agent, string) {
	t.Helper()
	tmp := t.TempDir()
	// Unix socket paths are limited to ~104 bytes on macOS.
	runDir, err := os.MkdirTemp("/tmp", "cm-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(runDir) })
	sys := filepath.Join(tmp, "sys")
	os.MkdirAll(filepath.Join(sys, "sys/class/drm/card0-Virtual-1"), 0o755)
	os.WriteFile(filepath.Join(sys, "sys/class/drm/card0-Virtual-1/status"), []byte("connected\n"), 0o644)
	os.MkdirAll(filepath.Join(sys, "etc"), 0o755)
	os.WriteFile(filepath.Join(sys, "etc/os-release"), []byte("ID=castmill\nVERSION_ID=\"0.1.0\"\n"), 0o644)

	cfg := config.Defaults()
	cfg.Backend.URL = backendURL
	cfg.Device.Machine, cfg.Device.Compatible = "genericarm64", "castmill-os-genericarm64"
	cfg.Paths.DataRoot = filepath.Join(tmp, "data")
	cfg.Paths.DataDir = filepath.Join(tmp, "data/castmill")
	cfg.Paths.LogDir = filepath.Join(tmp, "data/log")
	cfg.Paths.RunDir = runDir
	os.MkdirAll(cfg.AgentConfigDir(), 0o755)
	os.Symlink("/usr/share/zoneinfo/Europe/Madrid", cfg.LocaltimePath())
	return cfg, sys
}

func start(t *testing.T, a *Agent) context.CancelFunc {
	t.Helper()
	a.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	if testing.Verbose() {
		a.Log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = a.Run(ctx); close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("agent did not stop")
		}
	})
	return cancel
}

func TestBridgeAndState(t *testing.T) {
	var probes int
	var mu sync.Mutex
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		probes++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()
	cfg, sys := setup(t, backend.URL)
	start(t, &Agent{Cfg: cfg, SysRoot: sys})

	p := dialPlayer(t, cfg.PlayerSocketPath(), "Virtual-1")

	// The player is told to load the Castmill device app from the backend.
	eventually(t, "player state", 5*time.Second, func() bool {
		s := p.lastState()
		return s != nil && s["player_url"] == backend.URL+"/" && s["os_version"] == "0.1.0"
	})
	eventually(t, "backend online", 5*time.Second, func() bool { s := p.lastState(); return s["online"] == true })

	// machine_id: stable, per output.
	id1 := p.call("machine_id")["data"].(string)
	if len(id1) != 32+len("-Virtual-1") || id1[32:] != "-Virtual-1" {
		t.Fatalf("machine_id = %q", id1)
	}
	if id2 := p.call("machine_id")["data"]; id2 != id1 {
		t.Fatalf("machine_id not stable: %v vs %v", id2, id1)
	}

	info := p.call("device_info")["data"].(map[string]any)
	if info["appType"] != "Castmill OS" || info["os"] != "Castmill OS 0.1.0" || info["hardware"] != "genericarm64" {
		t.Fatalf("device_info = %v", info)
	}
	if tz := p.call("timezone")["data"]; tz != "Europe/Madrid" {
		t.Fatalf("timezone = %v", tz)
	}
	if tel := p.call("telemetry"); tel["ok"] != true {
		t.Fatalf("telemetry = %v", tel)
	}
	if r := p.call("reboot"); r["ok"] == true || r["error"] != "reboot not available" {
		t.Fatalf("reboot without systemd = %v", r)
	}
	if r := p.call("format_disk"); r["ok"] == true {
		t.Fatalf("unknown method must fail: %v", r)
	}

	// Runtime state for castmill-health: frames and backend reachability.
	go func() {
		for i := 0; i < 20; i++ {
			p.send(map[string]any{"type": "frame", "output": "Virtual-1"})
			time.Sleep(100 * time.Millisecond)
		}
	}()
	eventually(t, "runtime state", 10*time.Second, func() bool {
		rt, err := state.LoadRuntime(cfg.RuntimeStatePath())
		return err == nil && !rt.BackendOK.IsZero() && rt.Outputs["Virtual-1"].Player &&
			time.Since(rt.Outputs["Virtual-1"].LastFrame) < 5*time.Second
	})

	// The device ID survives an agent restart.
	b, _ := os.ReadFile(filepath.Join(cfg.StateDir(), "machine-id"))
	if string(b[:32]) != id1[:32] {
		t.Fatal("machine-id not persisted")
	}
}

func TestBackendDown(t *testing.T) {
	cfg, sys := setup(t, "http://127.0.0.1:1") // nothing listens
	start(t, &Agent{Cfg: cfg, SysRoot: sys})
	p := dialPlayer(t, cfg.PlayerSocketPath(), "HDMI-A-1")
	eventually(t, "backend error on screen", 10*time.Second, func() bool {
		s := p.lastState()
		return s != nil && s["online"] == false && s["backend_error"] != nil
	})
}

func TestDesiredUpdateInstalls(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()
	cfg, sys := setup(t, backend.URL)
	if err := config.WriteJSONAtomic(DesiredPath(cfg), api.Desired{Version: "0.2.0", BundleURL: "https://cdn.example/b.raucb"}, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &fakeRAUC{}
	start(t, &Agent{Cfg: cfg, SysRoot: sys, RAUC: r})
	eventually(t, "install", 5*time.Second, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return len(r.installs) == 1 && r.installs[0] == "https://cdn.example/b.raucb"
	})
	eventually(t, "installed state", 5*time.Second, func() bool {
		return state.LoadUpdate(filepath.Join(cfg.StateDir(), "update.json")).State == api.UpdateInstalled
	})
}
