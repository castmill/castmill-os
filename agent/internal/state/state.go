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
}

// Runtime is published by the running agent.
type Runtime struct {
	Updated      time.Time              `json:"updated"`
	AgentVersion string                 `json:"agent_version"`
	Fingerprint  string                 `json:"fingerprint"`
	Enrolled     bool                   `json:"enrolled"`
	DeviceID     string                 `json:"device_id,omitempty"`
	PairingCode  string                 `json:"pairing_code,omitempty"`
	BackendURL   string                 `json:"backend_url"`
	BackendOK    time.Time              `json:"backend_ok,omitzero"` // last successful request
	BackendError string                 `json:"backend_error,omitempty"`
	Connected    bool                   `json:"connected"` // WebSocket up
	TimeTrusted  bool                   `json:"time_trusted"`
	Outputs      map[string]OutputState `json:"outputs,omitempty"`
	Update       api.UpdateStatus       `json:"update"`
	LastError    string                 `json:"last_error,omitempty"`
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
