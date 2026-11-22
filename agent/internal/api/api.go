// Package api holds the types shared by castmill-agent's roles (daemon,
// health check, provisioning, session configuration) and persisted under
// /data. It is not a network protocol: Castmill devices talk to the backend
// through Castmill's own device app (docs/castmill-integration.md).
package api

import "time"

// Activation policies for a desired OS version.
const (
	ActivateWindow   = "window"   // reboot inside the maintenance window (default)
	ActivateNow      = "now"      // operator forced "apply now"
	ActivateSecurity = "security" // per-organisation security policy
)

// Desired is the OS version the device should run.
type Desired struct {
	Version        string `json:"version,omitempty"` // empty: nothing to do
	BundleURL      string `json:"bundle_url,omitempty"`
	Security       bool   `json:"security,omitempty"`
	ActivatePolicy string `json:"activate_policy,omitempty"`
	Pinned         bool   `json:"pinned,omitempty"`
}

// Update states.
const (
	UpdateIdle       = "idle"
	UpdateInstalling = "installing"
	UpdateInstalled  = "installed" // written to inactive slot, waiting for activation
	UpdateActivating = "activating"
	UpdateRunning    = "running" // booted the new version and committed it
	UpdateRolledBack = "rolled_back"
	UpdateFailed     = "failed"
)

// UpdateStatus describes the update state machine.
type UpdateStatus struct {
	State     string    `json:"state"`
	Version   string    `json:"version,omitempty"`
	Progress  int       `json:"progress,omitempty"` // 0-100
	LastError string    `json:"last_error,omitempty"`
	Updated   time.Time `json:"updated,omitzero"`
}

// HealthReport is the result of castmill-health.
type HealthReport struct {
	Time      time.Time     `json:"time"`
	Slot      string        `json:"slot,omitempty"`
	Version   string        `json:"version,omitempty"`
	Trial     bool          `json:"trial"`  // boot into an uncommitted slot
	Result    string        `json:"result"` // pass / fail / pending
	Checks    []HealthCheck `json:"checks"`
	Committed bool          `json:"committed"`
}

// HealthCheck is one item of a HealthReport.
type HealthCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// OutputConfig is the per-output display configuration.
type OutputConfig struct {
	Mode      string  `json:"mode,omitempty"`      // "1920x1080@60", or "preferred"
	Transform string  `json:"transform,omitempty"` // normal, rotate-90, rotate-180, rotate-270
	Scale     float64 `json:"scale,omitempty"`
	Disabled  bool    `json:"disabled,omitempty"`
}

// Window is a maintenance window in device-local time.
type Window struct {
	Days  []string `json:"days,omitempty"` // mon..sun; empty = every day
	Start string   `json:"start"`          // "03:00"
	End   string   `json:"end"`            // "05:00" (may be earlier than start: crosses midnight)
}

// DeviceConfig is the device-level configuration stored under /data.
type DeviceConfig struct {
	MaintenanceWindows []Window                `json:"maintenance_windows,omitempty"`
	SecurityPolicy     string                  `json:"security_policy,omitempty"` // "window" (default) or "24h"
	Outputs            map[string]OutputConfig `json:"outputs,omitempty"`
}
