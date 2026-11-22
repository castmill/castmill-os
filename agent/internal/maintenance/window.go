// Package maintenance decides when a device may reboot into a new version
// (UPD-003, spec 03 "Maintenance windows").
package maintenance

import (
	"fmt"
	"strings"
	"time"

	"github.com/castmill/castmill-os/agent/internal/api"
)

var dayNames = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

// DefaultWindows applies when the backend hasn't configured any: 03:00-05:00.
var DefaultWindows = []api.Window{{Start: "03:00", End: "05:00"}}

func parseClock(s string) (int, error) {
	var h, m int
	if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil || h < 0 || h > 24 || m < 0 || m > 59 || (h == 24 && m != 0) {
		return 0, fmt.Errorf("invalid time of day %q", s)
	}
	return h*60 + m, nil
}

// Validate checks the windows' syntax.
func Validate(ws []api.Window) error {
	for _, w := range ws {
		if _, err := parseClock(w.Start); err != nil {
			return err
		}
		if _, err := parseClock(w.End); err != nil {
			return err
		}
		for _, d := range w.Days {
			if _, ok := dayNames[strings.ToLower(d)]; !ok {
				return fmt.Errorf("invalid day %q", d)
			}
		}
	}
	return nil
}

// In reports whether t (already in device-local time) is inside any window.
// A window whose end is before its start crosses midnight; its Days refer to
// the day it starts. Start == End means the whole day.
func In(ws []api.Window, t time.Time) bool {
	if len(ws) == 0 {
		ws = DefaultWindows
	}
	now := t.Hour()*60 + t.Minute()
	for _, w := range ws {
		s, err1 := parseClock(w.Start)
		e, err2 := parseClock(w.End)
		if err1 != nil || err2 != nil {
			continue
		}
		switch {
		case s == e:
			if dayOK(w, t.Weekday()) {
				return true
			}
		case s < e:
			if now >= s && now < e && dayOK(w, t.Weekday()) {
				return true
			}
		default: // crosses midnight
			if now >= s && dayOK(w, t.Weekday()) {
				return true
			}
			if now < e && dayOK(w, (t.Weekday()+6)%7) {
				return true
			}
		}
	}
	return false
}

func dayOK(w api.Window, d time.Weekday) bool {
	if len(w.Days) == 0 {
		return true
	}
	for _, n := range w.Days {
		if dayNames[strings.ToLower(n)] == d {
			return true
		}
	}
	return false
}

// Decision explains whether to activate now.
type Decision struct {
	Activate bool
	Reason   string
}

// Policy inputs for ShouldActivate.
type Policy struct {
	Windows        []api.Window
	Location       *time.Location
	TimeTrusted    bool
	ActivatePolicy string        // api.Activate*
	SecurityPolicy string        // "window" or "24h"
	InstalledAt    time.Time     // when the bundle finished installing
	SecurityMaxAge time.Duration // for SecurityPolicy "24h"
}

// ShouldActivate decides whether an installed update may be activated (reboot)
// at time now.
func ShouldActivate(p Policy, now time.Time) Decision {
	if p.ActivatePolicy == api.ActivateNow {
		return Decision{true, "forced by operator"}
	}
	if !p.TimeTrusted {
		// Spec 05: with no valid time, activation is postponed.
		return Decision{false, "time not trusted yet"}
	}
	if p.ActivatePolicy == api.ActivateSecurity && p.SecurityPolicy == "24h" {
		max := p.SecurityMaxAge
		if max == 0 {
			max = 24 * time.Hour
		}
		if !p.InstalledAt.IsZero() && now.Sub(p.InstalledAt) >= max {
			return Decision{true, "security release past its deadline"}
		}
	}
	loc := p.Location
	if loc == nil {
		loc = time.UTC
	}
	if In(p.Windows, now.In(loc)) {
		return Decision{true, "inside maintenance window"}
	}
	return Decision{false, "outside maintenance window"}
}
