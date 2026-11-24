package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/castmill/castmill-os/agent/internal/wifi"
)

type fakeWiFi struct {
	mu      sync.Mutex
	online  bool
	joined  []string
	fail    bool
	noWiFi  bool // e.g. Ethernet only
	current *wifi.Network
}

func (f *fakeWiFi) Current(context.Context) (*wifi.Network, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.noWiFi {
		return nil, wifi.ErrNoWiFi
	}
	return f.current, nil
}

func (f *fakeWiFi) Online(context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.online, nil
}

func (f *fakeWiFi) Scan(context.Context) ([]wifi.Network, error) {
	return []wifi.Network{{SSID: "Venue", Signal: 80, Secure: true}}, nil
}

func (f *fakeWiFi) Join(_ context.Context, ssid, _ string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New(`couldn't join "Venue" (wrong password, or out of range?)`)
	}
	f.joined = append(f.joined, ssid)
	return nil
}

func TestOnScreenWiFi(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()
	cfg, sys := setup(t, backend.URL)
	w := &fakeWiFi{}
	start(t, &Agent{Cfg: cfg, SysRoot: sys, WiFi: w})
	p := dialPlayer(t, cfg.PlayerSocketPath(), "HDMI-A-1")

	// Offline: networks are listed and a network can be joined.
	st, _ := p.callArgs("wifi_status", nil)["data"].(map[string]any)
	nets, _ := st["networks"].([]any)
	if st["available"] != true || st["online"] != false || len(nets) != 1 {
		t.Fatalf("status = %v", st)
	}
	if r := p.callArgs("wifi_connect", map[string]any{"ssid": "Venue", "psk": "short"}); r["ok"] == true {
		t.Fatal("too short password accepted")
	}
	if r := p.callArgs("wifi_connect", map[string]any{"ssid": "Venue", "psk": "password123"}); r["ok"] != true {
		t.Fatalf("connect: %v", r)
	}
	w.fail = true
	if r := p.callArgs("wifi_connect", map[string]any{"ssid": "Venue", "psk": "wrongpass1"}); r["ok"] == true ||
		r["error"] != `couldn't join "Venue" (wrong password, or out of range?)` {
		t.Fatalf("failed join: %v", r)
	}

	// Online: the current network, no networks listed and no changes.
	w.mu.Lock()
	w.online, w.fail = true, false
	w.current = &wifi.Network{SSID: "Venue", Signal: 70, Secure: true, Active: true}
	w.mu.Unlock()
	st, _ = p.callArgs("wifi_status", nil)["data"].(map[string]any)
	if nets, _ := st["networks"].([]any); st["online"] != true || len(nets) != 0 {
		t.Fatalf("online status = %v", st)
	}
	if cur, _ := st["current"].(map[string]any); cur["ssid"] != "Venue" || cur["signal"] != float64(70) {
		t.Fatalf("current = %v", st["current"])
	}
	if r := p.callArgs("wifi_connect", map[string]any{"ssid": "Other", "psk": "password123"}); r["ok"] == true {
		t.Fatal("Wi-Fi changed while online")
	}
	if len(w.joined) != 1 {
		t.Fatalf("joined = %v", w.joined)
	}
}

func TestNoWiFi(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()
	cfg, sys := setup(t, backend.URL)
	start(t, &Agent{Cfg: cfg, SysRoot: sys})
	p := dialPlayer(t, cfg.PlayerSocketPath(), "HDMI-A-1")
	if st, _ := p.callArgs("wifi_status", nil)["data"].(map[string]any); st["available"] != false {
		t.Fatalf("status = %v", st)
	}
}

// Online over Ethernet without a Wi-Fi adapter: no Wi-Fi to offer.
func TestWiFiWithoutAdapter(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()
	cfg, sys := setup(t, backend.URL)
	start(t, &Agent{Cfg: cfg, SysRoot: sys, WiFi: &fakeWiFi{online: true, noWiFi: true}})
	p := dialPlayer(t, cfg.PlayerSocketPath(), "HDMI-A-1")
	if st, _ := p.callArgs("wifi_status", nil)["data"].(map[string]any); st["available"] != false {
		t.Fatalf("status = %v", st)
	}
}
