package agent

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/castmill/castmill-os/agent/internal/api"
	"github.com/castmill/castmill-os/agent/internal/config"
)

func TestCompareVersions(t *testing.T) {
	ordered := []string{
		"0.1.0", "0.2.0-dev.3.gabc1234", "0.2.0-rc.1", "0.2.0-rc.2", "0.2.0-rc.10",
		"0.2.0", "0.2.1-dev.1.gdef5678", "0.2.1", "0.10.0", "1.0.0",
	}
	for i := range ordered {
		for j := range ordered {
			want := 0
			switch {
			case i < j:
				want = -1
			case i > j:
				want = 1
			}
			if got := compareVersions(ordered[i], ordered[j]); got != want {
				t.Errorf("compare(%s, %s) = %d, want %d", ordered[i], ordered[j], got, want)
			}
		}
	}
	if compareVersions("v0.3.0", "0.3.0+build.5") != 0 {
		t.Error("v prefix and build metadata must not matter")
	}
	for _, bad := range []string{"", "1.2", "1.2.3.4", "01.2.3", "1.2.3-", "x.y.z"} {
		if _, ok := parseVersion(bad); ok {
			t.Errorf("%q accepted as a version", bad)
		}
	}
}

func TestOSUpdateFromCastmill(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()
	cfg, sys := setup(t, backend.URL) // the device runs 0.1.0
	cfg.Device.Compatible = "castmill-os-genericx86-64"
	r := &fakeRAUC{}
	start(t, &Agent{Cfg: cfg, SysRoot: sys, RAUC: r})
	p := dialPlayer(t, cfg.PlayerSocketPath(), "HDMI-A-1")
	url := "https://github.com/castmill/castmill-os/releases/download/v0.3.0/castmill-os-bundle-genericx86-64.raucb"

	// Rejected targets.
	for name, target := range map[string]map[string]any{
		"not a version":      {"version": "latest", "url": url},
		"plain http":         {"version": "0.3.0", "url": "http://example.com/b.raucb"},
		"bad activation":     {"version": "0.3.0", "url": url, "activate": "tomorrow"},
		"older, no override": {"version": "0.0.9", "url": url},
	} {
		if r := p.callArgs("set_os_update", target); r["ok"] == true {
			t.Errorf("%s: accepted", name)
		}
	}

	// A newer release: stored as the desired version and installed.
	if r := p.callArgs("set_os_update", map[string]any{"version": "0.3.0", "url": url, "sha256": "ab", "size": 1}); r["ok"] != true {
		t.Fatalf("set_os_update: %v", r)
	}
	var d api.Desired
	if err := config.ReadJSON(DesiredPath(cfg), &d); err != nil || d.Version != "0.3.0" ||
		d.BundleURL != url || d.ActivatePolicy != api.ActivateWindow {
		t.Fatalf("desired = %+v, %v", d, err)
	}
	eventually(t, "install", 5*time.Second, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return len(r.installs) == 1 && r.installs[0] == url
	})
	eventually(t, "installed status", 5*time.Second, func() bool {
		s := p.callArgs("os_update_status", nil)
		res, _ := s["data"].(map[string]any)
		return res["state"] == api.UpdateInstalled && res["version"] == "0.3.0" &&
			res["current_version"] == "0.1.0" && res["target_version"] == "0.3.0" &&
			res["compatible"] == "castmill-os-genericx86-64"
	})

	// A security release activates without waiting for the window.
	if r := p.callArgs("set_os_update", map[string]any{"version": "0.3.1", "url": url, "security": true}); r["ok"] != true {
		t.Fatalf("security target: %v", r)
	}
	if err := config.ReadJSON(DesiredPath(cfg), &d); err != nil || d.ActivatePolicy != api.ActivateSecurity {
		t.Fatalf("security desired = %+v", d)
	}

	// A device override may downgrade; "no update" clears the target.
	if r := p.callArgs("set_os_update", map[string]any{"version": "0.0.9", "url": url, "override": true, "activate": "now"}); r["ok"] != true {
		t.Fatalf("override downgrade: %v", r)
	}
	if err := config.ReadJSON(DesiredPath(cfg), &d); err != nil || d.Version != "0.0.9" || d.ActivatePolicy != api.ActivateNow {
		t.Fatalf("override desired = %+v", d)
	}
	if r := p.callArgs("set_os_update", map[string]any{"version": nil}); r["ok"] != true {
		t.Fatalf("clear: %v", r)
	}
	var cleared api.Desired
	if err := config.ReadJSON(DesiredPath(cfg), &cleared); err != nil || cleared != (api.Desired{}) {
		t.Fatalf("cleared desired = %+v", cleared)
	}
}

func TestOSUpdateWithoutRAUC(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()
	cfg, sys := setup(t, backend.URL)
	start(t, &Agent{Cfg: cfg, SysRoot: sys})
	p := dialPlayer(t, cfg.PlayerSocketPath(), "HDMI-A-1")

	if r := p.callArgs("set_os_update", map[string]any{"version": "0.3.0", "url": "https://x/b.raucb"}); r["ok"] == true {
		t.Fatal("accepted without RAUC")
	}
	res, _ := p.callArgs("os_update_status", nil)["data"].(map[string]any)
	if res["state"] != "unsupported" || res["current_version"] != "0.1.0" {
		t.Fatalf("status = %v", res)
	}
}
