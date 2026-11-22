package agent

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/castmill/castmill-os/agent/internal/api"
	"github.com/castmill/castmill-os/agent/internal/config"
	"github.com/castmill/castmill-os/agent/internal/mockbackend"
	"github.com/castmill/castmill-os/agent/internal/playerlink"
	"github.com/castmill/castmill-os/agent/internal/state"
)

// fakePlayer is a minimal stand-in for one Electron player instance.
type fakePlayer struct {
	t       *testing.T
	conn    net.Conn
	mu      sync.Mutex
	states  []playerlink.PlayerState
	reloads int
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
	p := &fakePlayer{t: t, conn: c}
	p.send(map[string]any{"type": "hello", "output": output})
	p.send(map[string]any{"type": "capabilities", "data": map[string]any{"video_decode": []string{"h264"}}})
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
		var m struct {
			Type  string                  `json:"type"`
			ID    string                  `json:"id"`
			State *playerlink.PlayerState `json:"state"`
		}
		_ = json.Unmarshal(sc.Bytes(), &m)
		p.mu.Lock()
		switch m.Type {
		case "state":
			p.states = append(p.states, *m.State)
		case "reload":
			p.reloads++
		case "screenshot":
			go p.send(map[string]any{"type": "screenshot", "id": m.ID, "png": base64.StdEncoding.EncodeToString([]byte("\x89PNG fake"))})
		}
		p.mu.Unlock()
	}
}

func (p *fakePlayer) last() playerlink.PlayerState {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.states) == 0 {
		return playerlink.PlayerState{}
	}
	return p.states[len(p.states)-1]
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

func TestEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("end-to-end")
	}
	tmp := t.TempDir()
	// Unix socket paths are limited to ~104 bytes on macOS.
	runDir, err := os.MkdirTemp("/tmp", "cm-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(runDir)

	serverCA, _ := mockbackend.NewCA("server")
	deviceCA, _ := mockbackend.NewCA("device")
	cert, _ := serverCA.ServerCert([]string{"127.0.0.1"})
	uploads := filepath.Join(tmp, "uploads")
	os.MkdirAll(uploads, 0o755)
	mb := mockbackend.New(mockbackend.Options{DeviceCA: deviceCA, UploadDir: uploads})
	srv := httptest.NewUnstartedServer(mb.DeviceHandler())
	srv.TLS = mb.TLSConfig(cert)
	srv.StartTLS()
	defer srv.Close()

	sys := filepath.Join(tmp, "sys")
	os.MkdirAll(filepath.Join(sys, "sys/class/drm/card0-Virtual-1"), 0o755)
	os.WriteFile(filepath.Join(sys, "sys/class/drm/card0-Virtual-1/status"), []byte("connected\n"), 0o644)
	os.MkdirAll(filepath.Join(sys, "etc"), 0o755)
	os.WriteFile(filepath.Join(sys, "etc/os-release"), []byte("ID=castmill\nVERSION_ID=\"2026.11.0\"\n"), 0o644)
	caFile := filepath.Join(tmp, "backend-ca.pem")
	os.WriteFile(caFile, serverCA.PEM, 0o644)

	cfg := config.Defaults()
	cfg.Backend.URL, cfg.Backend.CAFile = srv.URL, caFile
	cfg.Device.Machine, cfg.Device.Compatible = "genericarm64", "castmill-os-genericarm64"
	cfg.Paths.DataRoot = filepath.Join(tmp, "data")
	cfg.Paths.DataDir = filepath.Join(tmp, "data/castmill")
	cfg.Paths.LogDir = filepath.Join(tmp, "data/log")
	cfg.Paths.RunDir = runDir
	cfg.Player.DefaultURL = "https://player.example/?device={device_id}"
	os.MkdirAll(cfg.Paths.DataRoot, 0o755)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if testing.Verbose() {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &Agent{Cfg: cfg, Log: log, SysRoot: sys}
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()

	p := dialPlayer(t, cfg.PlayerSocketPath(), "Virtual-1")
	stopFrames := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopFrames:
				return
			case <-time.After(100 * time.Millisecond):
				p.send(map[string]any{"type": "frame", "output": "Virtual-1"})
			}
		}
	}()
	defer close(stopFrames)

	// 1. Pairing code shown on screen, operator approves it.
	eventually(t, "pairing code on screen", 10*time.Second, func() bool { return p.last().PairingCode != "" })
	if p.last().Enrolled || p.last().PlayerURL != "" {
		t.Fatalf("unexpected state before approval: %+v", p.last())
	}
	if err := mb.Approve(p.last().PairingCode, "stores/test"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "player switches to content", 15*time.Second, func() bool {
		s := p.last()
		return s.Enrolled && strings.HasPrefix(s.PlayerURL, "https://player.example/?device=dev-")
	})
	devID := p.last().DeviceID

	// 2. Device online, status posted with decode capabilities.
	eventually(t, "device online with status", 10*time.Second, func() bool {
		for _, d := range mb.Devices() {
			if d.ID == devID && d.Online && d.Status != nil && d.Status.OSVersion == "2026.11.0" && d.Status.DecodeCaps != nil {
				return true
			}
		}
		return false
	})

	// 3. Runtime state for castmill-health.
	eventually(t, "runtime state", 10*time.Second, func() bool {
		rt, err := state.LoadRuntime(cfg.RuntimeStatePath())
		return err == nil && rt.Enrolled && !rt.BackendOK.IsZero() &&
			time.Since(rt.Outputs["Virtual-1"].LastFrame) < 5*time.Second
	})

	// 4. Commands.
	cctx, ccancel := context.WithTimeout(ctx, 20*time.Second)
	defer ccancel()
	res, err := mb.Command(cctx, devID, api.CmdScreenshot, nil)
	if err != nil || !res.OK {
		t.Fatalf("screenshot: %v %+v", err, res)
	}
	up, _ := res.Data["uploads"].(map[string]any)
	if b, err := os.ReadFile(filepath.Join(uploads, up["Virtual-1"].(string))); err != nil || string(b) != "\x89PNG fake" {
		t.Fatalf("uploaded screenshot: %q %v", b, err)
	}
	if res, err := mb.Command(cctx, devID, api.CmdReloadPlayer, nil); err != nil || !res.OK {
		t.Fatalf("reload: %v %+v", err, res)
	}
	eventually(t, "player reloaded", 5*time.Second, func() bool { p.mu.Lock(); defer p.mu.Unlock(); return p.reloads == 1 })
	if res, _ := mb.Command(cctx, devID, "format_disk", nil); res == nil || res.OK {
		t.Fatalf("unknown command must fail: %+v", res)
	}
	if res, _ := mb.Command(cctx, devID, api.CmdFactoryReset, map[string]any{"confirm": "wrong"}); res == nil || res.OK {
		t.Fatalf("factory reset without confirmation must fail: %+v", res)
	}

	// 5. Configuration push.
	if err := mb.SetConfig(devID, api.DeviceConfig{PlayerURL: "https://other.example/{device_id}",
		Outputs: map[string]api.OutputConfig{"Virtual-1": {Transform: "rotate-90"}}}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "new player URL", 10*time.Second, func() bool { return p.last().PlayerURL == "https://other.example/"+devID })
	var disp map[string]api.OutputConfig
	eventually(t, "display config", 5*time.Second, func() bool {
		return config.ReadJSON(cfg.DisplayConfigPath(), &disp) == nil && disp["Virtual-1"].Transform == "rotate-90"
	})

	// 6. Revocation: the backend rejects the device; the screen explains it.
	mb.Revoke(devID)
	eventually(t, "revoked message", 15*time.Second, func() bool { return strings.Contains(p.last().BackendError, "revoked") })

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("agent did not stop")
	}
}
