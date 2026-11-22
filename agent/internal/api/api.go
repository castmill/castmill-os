// Package api defines the wire types of the Castmill OS device API
// (agent <-> Castmill backend). See docs/device-api.md.
package api

import "time"

// Base path of the device API.
const Prefix = "/api/os/v1"

// Enrollment states.
const (
	EnrollmentPending  = "pending"
	EnrollmentApproved = "approved"
	EnrollmentRejected = "rejected"
	EnrollmentExpired  = "expired"
)

// Hardware describes the device during enrollment.
type Hardware struct {
	Machine    string `json:"machine"`    // Yocto MACHINE, e.g. genericx86-64
	Compatible string `json:"compatible"` // RAUC compatible string
	Model      string `json:"model,omitempty"`
	Serial     string `json:"serial,omitempty"`
	OSVersion  string `json:"os_version"`
	DevImage   bool   `json:"dev_image"`
}

// EnrollmentRequest starts an enrollment. Sent over server-authenticated TLS
// (the device has no certificate yet).
type EnrollmentRequest struct {
	CSR      string   `json:"csr"` // PEM PKCS#10, public key of the device identity
	Hardware Hardware `json:"hardware"`
	OrgToken string   `json:"org_token,omitempty"` // zero-touch bulk enrollment
	Group    string   `json:"group,omitempty"`
	Name     string   `json:"name,omitempty"`
}

// Enrollment is the server's view of an enrollment.
type Enrollment struct {
	ID            string    `json:"id"`
	Status        string    `json:"status"`
	PairingCode   string    `json:"pairing_code,omitempty"`
	PollIntervalS int       `json:"poll_interval_s,omitempty"`
	ExpiresAt     time.Time `json:"expires_at,omitzero"`
	DeviceID      string    `json:"device_id,omitempty"`
	Certificate   string    `json:"certificate,omitempty"` // PEM, leaf first
	CA            string    `json:"ca,omitempty"`          // PEM, device CA (for reference)
	Reason        string    `json:"reason,omitempty"`
}

// Activation policies for a desired version.
const (
	ActivateWindow   = "window"   // reboot inside the maintenance window (default)
	ActivateNow      = "now"      // operator forced "apply now"
	ActivateSecurity = "security" // per-organisation security policy
)

// Desired is what the backend wants the device to run.
type Desired struct {
	Version        string `json:"version,omitempty"` // empty: nothing to do
	BundleURL      string `json:"bundle_url,omitempty"`
	SHA256         string `json:"sha256,omitempty"`
	Security       bool   `json:"security,omitempty"`
	ActivatePolicy string `json:"activate_policy,omitempty"`
	Pinned         bool   `json:"pinned,omitempty"`
}

// Update states reported by the agent.
const (
	UpdateIdle       = "idle"
	UpdateInstalling = "installing"
	UpdateInstalled  = "installed" // written to inactive slot, waiting for activation
	UpdateActivating = "activating"
	UpdateRunning    = "running" // booted the new version, health pending or ok
	UpdateRolledBack = "rolled_back"
	UpdateFailed     = "failed"
)

// UpdateStatus is the update part of DeviceStatus.
type UpdateStatus struct {
	State     string    `json:"state"`
	Version   string    `json:"version,omitempty"`
	Progress  int       `json:"progress,omitempty"` // 0-100
	LastError string    `json:"last_error,omitempty"`
	Updated   time.Time `json:"updated,omitzero"`
}

// Slot is a RAUC slot as reported to the backend.
type Slot struct {
	Name          string `json:"name"`
	Bootname      string `json:"bootname,omitempty"`
	State         string `json:"state,omitempty"`       // booted / active / inactive
	BootStatus    string `json:"boot_status,omitempty"` // good / bad
	BundleVersion string `json:"bundle_version,omitempty"`
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

// Output is a display output as seen by the device.
type Output struct {
	Name      string    `json:"name"` // DRM connector, e.g. HDMI-A-1
	Connected bool      `json:"connected"`
	LastFrame time.Time `json:"last_frame,omitzero"`
}

// DeviceStatus is POSTed to /devices/{id}/status.
type DeviceStatus struct {
	OSVersion    string         `json:"os_version"`
	AgentVersion string         `json:"agent_version"`
	Machine      string         `json:"machine"`
	Compatible   string         `json:"compatible"`
	BootedSlot   string         `json:"booted_slot,omitempty"`
	Slots        []Slot         `json:"slots,omitempty"`
	Update       UpdateStatus   `json:"update"`
	Health       *HealthReport  `json:"health,omitempty"`
	Outputs      []Output       `json:"outputs,omitempty"`
	TimeTrusted  bool           `json:"time_trusted"`
	DecodeCaps   map[string]any `json:"decode_capabilities,omitempty"` // MED-007, from the player
	Uptime       float64        `json:"uptime_s"`
}

// Telemetry is sent periodically over the WebSocket.
type Telemetry struct {
	Time         time.Time          `json:"time"`
	CPUPercent   float64            `json:"cpu_percent"`
	Load1        float64            `json:"load1"`
	MemTotalKB   uint64             `json:"mem_total_kb"`
	MemAvailKB   uint64             `json:"mem_available_kb"`
	Temperatures map[string]float64 `json:"temperatures_c,omitempty"`
	Throttled    string             `json:"throttled,omitempty"` // Raspberry Pi get_throttled bitmask (hex)
	DataTotalB   uint64             `json:"data_total_bytes"`
	DataFreeB    uint64             `json:"data_free_bytes"`
	StorageWear  map[string]string  `json:"storage_wear,omitempty"`
	UptimeS      float64            `json:"uptime_s"`
	Player       map[string]any     `json:"player,omitempty"` // per-output dropped frames etc.
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

// DeviceConfig is pushed by the backend (WebSocket "config" message or
// GET /devices/{id}/config) and stored under /data.
type DeviceConfig struct {
	Revision           int64                   `json:"revision"`
	Timezone           string                  `json:"timezone,omitempty"`
	MaintenanceWindows []Window                `json:"maintenance_windows,omitempty"`
	SecurityPolicy     string                  `json:"security_policy,omitempty"` // "window" (default) or "24h"
	PlayerURL          string                  `json:"player_url,omitempty"`
	Outputs            map[string]OutputConfig `json:"outputs,omitempty"`
	TelemetryIntervalS int                     `json:"telemetry_interval_s,omitempty"`
}

// WebSocket message types.
const (
	MsgHello     = "hello"     // device -> server, first message
	MsgCommand   = "command"   // server -> device
	MsgResult    = "result"    // device -> server, answer to a command
	MsgConfig    = "config"    // server -> device
	MsgDesired   = "desired"   // server -> device: new desired version available
	MsgTelemetry = "telemetry" // device -> server
	MsgStatus    = "status"    // device -> server
)

// Commands (MGT-001).
const (
	CmdReboot        = "reboot"
	CmdRestartPlayer = "restart_player"
	CmdReloadPlayer  = "reload_player"
	CmdScreenshot    = "screenshot"
	CmdCollectLogs   = "collect_logs"
	CmdFactoryReset  = "factory_reset"
	CmdCheckUpdate   = "check_update"
	CmdApplyNow      = "apply_update_now"
)

// Message is the WebSocket envelope.
type Message struct {
	Type      string         `json:"type"`
	ID        string         `json:"id,omitempty"`
	Command   string         `json:"command,omitempty"`
	Args      map[string]any `json:"args,omitempty"`
	OK        bool           `json:"ok,omitempty"`
	Error     string         `json:"error,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	Config    *DeviceConfig  `json:"config,omitempty"`
	Desired   *Desired       `json:"desired,omitempty"`
	Status    *DeviceStatus  `json:"status,omitempty"`
	Telemetry *Telemetry     `json:"telemetry,omitempty"`
}

// UploadResponse is returned by POST /devices/{id}/uploads.
type UploadResponse struct {
	ID string `json:"id"`
}

// CertificateRequest renews the device certificate (mTLS).
type CertificateRequest struct {
	CSR string `json:"csr"`
}

// CertificateResponse answers CertificateRequest.
type CertificateResponse struct {
	Certificate string `json:"certificate"`
}
