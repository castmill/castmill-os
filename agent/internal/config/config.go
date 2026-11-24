// Package config loads the agent configuration.
//
// Two files are involved:
//
//   - /etc/castmill/agent.toml: image defaults (read-only rootfs). Backend URL,
//     pinned backend CA, intervals, machine identity. Built into the image.
//   - castmill.conf: optional provisioning file (spec 08) found on the boot
//     partition or a USB stick. Consumed once by "castmill-agent provision".
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/castmill/castmill-os/agent/internal/wifi"
)

// DefaultPath is where the image installs the agent configuration.
const DefaultPath = "/etc/castmill/agent.toml"

// Duration is a time.Duration that decodes from TOML strings such as "15m".
type Duration struct{ time.Duration }

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

// Agent is the content of agent.toml.
type Agent struct {
	Backend struct {
		URL    string `toml:"url"`
		CAFile string `toml:"ca_file"` // pinned backend CA; empty = system roots
		// AllowOverride lets castmill.conf replace the backend URL and CA.
		// Only enabled in development images.
		AllowOverride bool `toml:"allow_override"`
	} `toml:"backend"`
	Device struct {
		Machine    string `toml:"machine"`
		Compatible string `toml:"compatible"`
		DevImage   bool   `toml:"dev_image"`
	} `toml:"device"`
	Paths struct {
		DataDir  string `toml:"data_dir"`  // /data/castmill
		RunDir   string `toml:"run_dir"`   // /run/castmill-agent
		LogDir   string `toml:"log_dir"`   // /data/log/castmill
		BootDir  string `toml:"boot_dir"`  // /boot (provisioning file, factory-reset flag)
		DataRoot string `toml:"data_root"` // /data (mount point; factory-reset flag)
	} `toml:"paths"`
	Player struct {
		// FrameTimeout: an output without a "frame rendered" heartbeat for this
		// long is reloaded, then restarted (spec 05, recovery layer 1).
		FrameTimeout Duration `toml:"frame_timeout"`
	} `toml:"player"`
	Health struct {
		// Timeout to commit a trial slot (spec 05: N minutes, default 10).
		Timeout Duration `toml:"timeout"`
		// CriticalUnits must be active for the health check to pass.
		CriticalUnits []string `toml:"critical_units"`
		// RequireOutput: at least one connected output with frames.
		RequireOutput bool `toml:"require_output"`
	} `toml:"health"`
}

// Defaults returns the built-in configuration.
func Defaults() *Agent {
	a := &Agent{}
	a.Backend.URL = "https://castmill.net"
	a.Paths.DataDir = "/data/castmill"
	a.Paths.RunDir = "/run/castmill-agent"
	a.Paths.LogDir = "/data/log/castmill"
	a.Paths.BootDir = "/boot"
	a.Paths.DataRoot = "/data"
	a.Player.FrameTimeout = Duration{30 * time.Second}
	a.Health.Timeout = Duration{10 * time.Minute}
	a.Health.CriticalUnits = []string{"castmill-agent.service", "castmill-session.service", "NetworkManager.service"}
	a.Health.RequireOutput = true
	return a
}

// Load reads agent.toml on top of the defaults. A missing file is not an error.
func Load(path string) (*Agent, error) {
	a := Defaults()
	if _, err := toml.DecodeFile(path, a); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	return a, nil
}

// Derived paths under DataDir / RunDir.
func (a *Agent) IdentityDir() string    { return filepath.Join(a.Paths.DataDir, "identity") }
func (a *Agent) ConfigDir() string      { return filepath.Join(a.Paths.DataDir, "config") }
func (a *Agent) AgentConfigDir() string { return filepath.Join(a.Paths.DataDir, "config", "agent") }
func (a *Agent) StateDir() string       { return filepath.Join(a.Paths.DataDir, "state") }

// RuntimeStatePath is the agent state shared with castmill-health (tmpfs).
func (a *Agent) RuntimeStatePath() string { return filepath.Join(a.Paths.RunDir, "state.json") }

// PlayerSocketPath is the Unix socket the player connects to.
func (a *Agent) PlayerSocketPath() string { return filepath.Join(a.Paths.RunDir, "player.sock") }

// HealthReportPath is the latest health report (persistent, small).
func (a *Agent) HealthReportPath() string { return filepath.Join(a.Paths.LogDir, "health.json") }

// ProvisioningPath holds the enrollment settings extracted from castmill.conf.
func (a *Agent) ProvisioningPath() string { return filepath.Join(a.ConfigDir(), "provisioning.json") }

// DeviceConfigPath holds the last DeviceConfig received from the backend.
func (a *Agent) DeviceConfigPath() string { return filepath.Join(a.AgentConfigDir(), "device.json") }

// DisplayConfigPath is read by the session script (per-output mode/rotation).
func (a *Agent) DisplayConfigPath() string { return filepath.Join(a.AgentConfigDir(), "display.json") }

// PlayerConfigPath is read by the player (URL etc.).
func (a *Agent) PlayerConfigPath() string { return filepath.Join(a.AgentConfigDir(), "player.json") }

// LocaltimePath is the target of the /etc/localtime symlink.
func (a *Agent) LocaltimePath() string { return filepath.Join(a.AgentConfigDir(), "localtime") }

// Provisioning is what "provision" keeps from castmill.conf for the agent.
// Secrets for the network are handed to NetworkManager and not stored here.
type Provisioning struct {
	OrgToken     string `json:"org_token,omitempty"`
	Group        string `json:"group,omitempty"`
	NameTemplate string `json:"name_template,omitempty"`
	BackendURL   string `json:"backend_url,omitempty"` // dev images only
	BackendCA    string `json:"backend_ca,omitempty"`  // PEM, dev images only
	Timezone     string `json:"timezone,omitempty"`
	WiFiCountry  string `json:"wifi_country,omitempty"`
}

// LoadProvisioning reads provisioning.json; missing file returns an empty value.
func LoadProvisioning(path string) (*Provisioning, error) {
	p := &Provisioning{}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	return p, json.Unmarshal(b, p)
}

// CastmillConf is the provisioning file format (spec 08).
type CastmillConf struct {
	Enrollment struct {
		OrgToken     string `toml:"org_token"`
		Group        string `toml:"group"`
		NameTemplate string `toml:"name_template"`
	} `toml:"enrollment"`
	Backend struct { // honoured on development images only
		URL        string `toml:"url"`
		CACertFile string `toml:"ca_cert_file"`
	} `toml:"backend"`
	Network struct {
		WiFi *struct {
			SSID   string `toml:"ssid"`
			PSK    string `toml:"psk"`
			Hidden bool   `toml:"hidden"`
		} `toml:"wifi"`
		// Wi-Fi country (ISO 3166-1 alpha-2, e.g. "SE"); default: the time
		// zone's country.
		WiFiCountry string `toml:"wifi_country"`
		Proxy       *struct {
			HTTP       string `toml:"http"`
			HTTPS      string `toml:"https"`
			NoProxy    string `toml:"no_proxy"`
			CACertFile string `toml:"ca_cert_file"`
		} `toml:"proxy"`
	} `toml:"network"`
	Time struct {
		NTP      []string `toml:"ntp"`
		Timezone string   `toml:"timezone"`
	} `toml:"time"`
}

// ParseCastmillConf decodes castmill.conf and rejects unknown keys, so a typo
// doesn't silently produce an unconfigured device.
func ParseCastmillConf(path string) (*CastmillConf, error) {
	c := &CastmillConf{}
	md, err := toml.DecodeFile(path, c)
	if err != nil {
		return nil, err
	}
	if und := md.Undecoded(); len(und) > 0 {
		return nil, fmt.Errorf("%s: unknown keys: %v", path, und)
	}
	if c.Network.WiFi != nil && c.Network.WiFi.SSID == "" {
		return nil, fmt.Errorf("%s: [network.wifi] needs ssid", path)
	}
	if cc := c.Network.WiFiCountry; cc != "" {
		c.Network.WiFiCountry = strings.ToUpper(cc)
		if !wifi.ValidCountry(c.Network.WiFiCountry) {
			return nil, fmt.Errorf("%s: [network] wifi_country %q is not a country code (ISO 3166), like \"SE\"", path, cc)
		}
	}
	return c, nil
}

// WriteFileAtomic writes data to path via a temp file + fsync + rename, so a
// power cut leaves either the old or the new content (REL-005).
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op after a successful rename
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}

// WriteJSONAtomic marshals v and writes it atomically.
func WriteJSONAtomic(path string, v any, perm os.FileMode) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(b, '\n'), perm)
}

// ReadJSON reads a JSON file into v. Missing file returns os.ErrNotExist.
func ReadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
