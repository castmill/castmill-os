package agent

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/castmill/castmill-os/agent/internal/state"
)

func TestGPUReport(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer backend.Close()
	cfg, sys := setup(t, backend.URL)
	start(t, &Agent{Cfg: cfg, SysRoot: sys})
	p := dialPlayer(t, cfg.PlayerSocketPath(), "Virtual-1")

	if r := p.callArgs("gpu_report", map[string]any{"rendering": "fast"}); r["ok"] == true {
		t.Fatalf("invalid rendering accepted: %v", r)
	}
	report := map[string]any{
		"rendering":    "hardware",
		"renderer":     "Mesa Intel(R) Graphics (ADL-N)",
		"video_decode": []string{"H.264", "HEVC", "VP9", "AV1"},
		"features":     map[string]string{"gpu_compositing": "enabled", "video_decode": "enabled"},
	}
	if r := p.callArgs("gpu_report", report); r["ok"] != true {
		t.Fatalf("gpu_report: %v", r)
	}
	var g *state.GPUState
	eventually(t, "GPU in the runtime state", 10*time.Second, func() bool {
		rt, err := state.LoadRuntime(cfg.RuntimeStatePath())
		if err != nil {
			return false
		}
		g = rt.Outputs["Virtual-1"].GPU
		return g != nil
	})
	if g.Rendering != "hardware" || g.Renderer != "Mesa Intel(R) Graphics (ADL-N)" ||
		!slices.Equal(g.VideoDecode, []string{"H.264", "HEVC", "VP9", "AV1"}) ||
		g.Features["gpu_compositing"] != "enabled" {
		t.Fatalf("GPU state = %+v", g)
	}
}
