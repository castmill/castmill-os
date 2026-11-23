// Package state holds the state the agent shares with other processes.
//
//   - Runtime (tmpfs, /run/castmill-agent/state.json): rewritten every few
//     seconds; read by castmill-health and "castmill-agent status".
//   - Update (persistent, /data/castmill/state/update.json): survives reboots
//     so the agent knows it is waiting for activation or verifying a new slot.
package state

import (
	"time"

	"github.com/castmill/castmill-os/agent/internal/api"
	"github.com/castmill/castmill-os/agent/internal/config"
)

// OutputState is the player's view of one output.
type OutputState struct {
	Connected bool      `json:"connected"`
	LastFrame time.Time `json:"last_frame,omitzero"`
	Player    bool      `json:"player"` // a player instance is attached
	// GPU is what the output's player reported about GPU acceleration.
	GPU *GPUState `json:"gpu,omitempty"`
}

// Rendering modes in GPUState.
const (
	RenderingHardware = "hardware"
	RenderingSoftware = "software"
)

// GPUState is a player's GPU acceleration, as Chromium sees it.
type GPUState struct {
	// Rendering is "hardware" when Chromium composites on the GPU,
	// "software" when it fell back to the CPU.
	Rendering string `json:"rendering"`
	// Renderer is the GL renderer, e.g. "Mesa Intel(R) Graphics (ADL-N)".
	Renderer string `json:"renderer,omitempty"`
	// VideoDecode lists the codecs decoded in hardware, e.g. ["H.264", "HEVC"].
	VideoDecode []string `json:"video_decode"`
	// Features is Chromium's feature status (chrome://gpu), e.g.
	// "gpu_compositing": "enabled".
	Features map[string]string `json:"features,omitempty"`
}

// Display power states.
const (
	PowerOn  = "on"
	PowerOff = "off"
)

// DisplayState is the display power the device schedule set on one output.
type DisplayState struct {
	Power   string    `json:"power"`            // PowerOn or PowerOff
	Since   time.Time `json:"since,omitzero"`   // scheduled change
	Method  string    `json:"method,omitempty"` // "cec", "ddc", "none"
	Error   string    `json:"error,omitempty"`
	RetryAt time.Time `json:"-"`
}

// Runtime is published by the running agent.
type Runtime struct {
	Updated      time.Time              `json:"updated"`
	AgentVersion string                 `json:"agent_version"`
	DeviceName   string                 `json:"device_name"`
	BackendURL   string                 `json:"backend_url"`
	BackendOK    time.Time              `json:"backend_ok,omitzero"` // last time the backend answered
	BackendError string                 `json:"backend_error,omitempty"`
	TimeTrusted  bool                   `json:"time_trusted"`
	Outputs      map[string]OutputState `json:"outputs,omitempty"`
	// Display is the scheduled display power per output (device schedule).
	Display map[string]DisplayState `json:"display,omitempty"`
	Update  api.UpdateStatus        `json:"update"`
}

// Update is the persistent update bookkeeping.
type Update struct {
	State          string    `json:"state"`
	Version        string    `json:"version,omitempty"`
	BundleURL      string    `json:"bundle_url,omitempty"`
	ActivatePolicy string    `json:"activate_policy,omitempty"`
	Security       bool      `json:"security,omitempty"`
	InstalledAt    time.Time `json:"installed_at,omitzero"`
	TargetSlot     string    `json:"target_slot,omitempty"` // bootname written by the install
	FromVersion    string    `json:"from_version,omitempty"`
	ActivatedAt    time.Time `json:"activated_at,omitzero"`
	LastError      string    `json:"last_error,omitempty"`
	// Reported is set once the final outcome (running / rolled back) has been
	// acknowledged by the backend.
	Reported bool `json:"reported,omitempty"`
}

// LoadUpdate reads the update state; missing file returns an idle state.
func LoadUpdate(path string) Update {
	var u Update
	if err := config.ReadJSON(path, &u); err != nil || u.State == "" {
		return Update{State: api.UpdateIdle}
	}
	return u
}

// SaveUpdate writes the update state atomically.
func SaveUpdate(path string, u Update) error {
	return config.WriteJSONAtomic(path, u, 0o644)
}

// LoadRuntime reads the runtime state.
func LoadRuntime(path string) (*Runtime, error) {
	r := &Runtime{}
	return r, config.ReadJSON(path, r)
}
