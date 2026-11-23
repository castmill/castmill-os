package agent

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/castmill/castmill-os/agent/internal/state"
)

// GPU report (MED-002 / MED-007). Each kiosk shell tells the agent, once its
// GPU process is up, whether Chromium renders and decodes video on the GPU.
// It's shown in "castmill-agent status" and logged, so a software fallback
// (missing driver, unsupported GPU) is visible on the device.

const maxGPUField = 200

func (a *Agent) gpuReport(output string, args json.RawMessage) error {
	var g state.GPUState
	if err := json.Unmarshal(args, &g); err != nil {
		return fmt.Errorf("gpu_report: %w", err)
	}
	if g.Rendering != state.RenderingHardware && g.Rendering != state.RenderingSoftware {
		return fmt.Errorf("gpu_report: rendering must be %q or %q", state.RenderingHardware, state.RenderingSoftware)
	}
	g.Renderer = clip(g.Renderer)
	if len(g.VideoDecode) > 16 {
		g.VideoDecode = g.VideoDecode[:16]
	}
	for i, c := range g.VideoDecode {
		g.VideoDecode[i] = clip(c)
	}
	features := map[string]string{}
	for k, v := range g.Features {
		if len(features) == 32 {
			break
		}
		features[clip(k)] = clip(v)
	}
	g.Features = features

	a.mu.Lock()
	if a.gpu == nil {
		a.gpu = map[string]state.GPUState{}
	}
	a.gpu[output] = g
	a.mu.Unlock()

	decode := "none"
	if len(g.VideoDecode) > 0 {
		decode = strings.Join(g.VideoDecode, ", ")
	}
	if g.Rendering == state.RenderingSoftware {
		a.Log.Warn("player renders in software (no GPU acceleration)", "output", output,
			"renderer", g.Renderer, "video_decode", decode)
	} else {
		a.Log.Info("player GPU", "output", output, "renderer", g.Renderer, "video_decode", decode)
	}
	return nil
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxGPUField {
		s = s[:maxGPUField]
	}
	return s
}

// gpuSnapshot copies the GPU reports for the runtime state (callers hold a.mu).
func (a *Agent) gpuSnapshot(output string) *state.GPUState {
	g, ok := a.gpu[output]
	if !ok {
		return nil
	}
	g.VideoDecode = slices.Clone(g.VideoDecode)
	return &g
}
