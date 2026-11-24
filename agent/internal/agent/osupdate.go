package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/castmill/castmill-os/agent/internal/api"
	"github.com/castmill/castmill-os/agent/internal/config"
)

// OS updates from Castmill (ADR-0010). Castmill's device app hands the
// target version it received on the device channel to the kiosk shell, which
// forwards it here (set_os_update); the device app reads the progress back
// with os_update_status and reports it to Castmill. The target is stored as
// the desired version, like `castmill-agent update` does.

// osUpdateTarget is the os_update message of the device channel.
type osUpdateTarget struct {
	Version  string `json:"version"` // empty or null: no update
	URL      string `json:"url"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	Security bool   `json:"security"`
	Activate string `json:"activate"` // "window" (default) or "now"
	// Override is set for a version chosen for this device; only an
	// override may install a version older than the running one.
	Override bool `json:"override"`
}

// OSUpdateStatus is what the device app reports to Castmill.
type OSUpdateStatus struct {
	Compatible     string    `json:"compatible"`
	CurrentVersion string    `json:"current_version"`
	TargetVersion  string    `json:"target_version,omitempty"`
	State          string    `json:"state"`
	Version        string    `json:"version,omitempty"` // version the state refers to
	Progress       int       `json:"progress,omitempty"`
	LastError      string    `json:"last_error,omitempty"`
	Updated        time.Time `json:"updated,omitzero"`
}

var errNoUpdates = errors.New("this device can't install updates (no RAUC)")

func (a *Agent) setOSUpdate(output string, args json.RawMessage) error {
	if a.upd == nil {
		return errNoUpdates
	}
	var t osUpdateTarget
	if err := json.Unmarshal(args, &t); err != nil {
		return fmt.Errorf("set_os_update: %w", err)
	}
	d := api.Desired{}
	if t.Version != "" {
		if _, ok := parseVersion(t.Version); !ok {
			return fmt.Errorf("set_os_update: %q is not a version", t.Version)
		}
		u, err := url.Parse(t.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("set_os_update: the bundle URL must be https")
		}
		if !t.Override && a.osVersion != "" && compareVersions(t.Version, a.osVersion) < 0 {
			return fmt.Errorf("set_os_update: %s is older than the running %s; only a device override can downgrade",
				t.Version, a.osVersion)
		}
		d = api.Desired{Version: t.Version, BundleURL: t.URL, Security: t.Security, ActivatePolicy: api.ActivateWindow}
		switch {
		case t.Activate == api.ActivateNow:
			d.ActivatePolicy = api.ActivateNow
		case t.Activate != "" && t.Activate != api.ActivateWindow:
			return fmt.Errorf("set_os_update: activate must be %q or %q", api.ActivateWindow, api.ActivateNow)
		case t.Security:
			d.ActivatePolicy = api.ActivateSecurity
		}
	}

	var cur api.Desired
	_ = config.ReadJSON(DesiredPath(a.Cfg), &cur)
	if cur == d {
		return nil // every output reports the same target; nothing changed
	}
	if err := config.WriteJSONAtomic(DesiredPath(a.Cfg), d, 0o644); err != nil {
		return err
	}
	if d.Version == "" {
		a.Log.Info("OS update target cleared by Castmill", "output", output)
	} else {
		a.Log.Info("OS update target from Castmill", "output", output, "version", d.Version,
			"activate", d.ActivatePolicy)
	}
	select {
	case a.updateKick <- struct{}{}:
	default:
	}
	return nil
}

func (a *Agent) osUpdateStatus() OSUpdateStatus {
	s := OSUpdateStatus{Compatible: a.Cfg.Device.Compatible, CurrentVersion: a.osVersion, State: api.UpdateIdle}
	var d api.Desired
	if config.ReadJSON(DesiredPath(a.Cfg), &d) == nil {
		s.TargetVersion = d.Version
	}
	if a.upd == nil {
		s.State = "unsupported"
		return s
	}
	st := a.upd.Status()
	s.State, s.Version, s.Progress, s.LastError, s.Updated = st.State, st.Version, st.Progress, st.LastError, st.Updated
	if s.State == "" {
		s.State = api.UpdateIdle
	}
	return s
}

// ---- versions (SemVer 2.0.0 precedence) --------------------------------------

type version struct {
	core [3]int
	pre  []string
}

func parseVersion(v string) (version, bool) {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i] // build metadata doesn't affect precedence
	}
	core, pre, hasPre := strings.Cut(v, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return version{}, false
	}
	var out version
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || (len(p) > 1 && p[0] == '0') {
			return version{}, false
		}
		out.core[i] = n
	}
	if hasPre {
		if pre == "" {
			return version{}, false
		}
		out.pre = strings.Split(pre, ".")
	}
	return out, true
}

// compareVersions returns -1, 0 or 1. Unparsable versions sort first.
func compareVersions(a, b string) int {
	va, okA := parseVersion(a)
	vb, okB := parseVersion(b)
	switch {
	case !okA && !okB:
		return strings.Compare(a, b)
	case !okA:
		return -1
	case !okB:
		return 1
	}
	for i := range 3 {
		if va.core[i] != vb.core[i] {
			if va.core[i] < vb.core[i] {
				return -1
			}
			return 1
		}
	}
	// A pre-release sorts before the release.
	switch {
	case len(va.pre) == 0 && len(vb.pre) == 0:
		return 0
	case len(va.pre) == 0:
		return 1
	case len(vb.pre) == 0:
		return -1
	}
	for i := 0; i < len(va.pre) && i < len(vb.pre); i++ {
		if c := comparePre(va.pre[i], vb.pre[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(va.pre) < len(vb.pre):
		return -1
	case len(va.pre) > len(vb.pre):
		return 1
	}
	return 0
}

func comparePre(a, b string) int {
	na, errA := strconv.Atoi(a)
	nb, errB := strconv.Atoi(b)
	switch {
	case errA == nil && errB == nil:
		switch {
		case na < nb:
			return -1
		case na > nb:
			return 1
		}
		return 0
	case errA == nil:
		return -1 // numeric identifiers sort before alphanumeric ones
	case errB == nil:
		return 1
	}
	return strings.Compare(a, b)
}
