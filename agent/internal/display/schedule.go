// Package display switches screens on and off on the device schedule set in
// the Castmill dashboard (spec 06 "Display power", MED-006).
//
// The schedule is Castmill's device timers (Timers in the Castmill repository's
// packages/device/src/interfaces/machine.ts). Castmill's device app blanks the
// screen and stops playback at those times; the agent additionally puts the
// display itself in standby, over HDMI-CEC or, for monitors, DDC/CI.
package display

import (
	"encoding/json"
	"fmt"
	"time"
)

// Entry is one on or off time: hours and minutes on some days of the week
// ("MON".."SUN", or "ALL").
type Entry struct {
	Hours    int      `json:"hours"`
	Minutes  int      `json:"minutes"`
	WeekDays []string `json:"weekDays"`
}

// Timers is a device schedule.
type Timers struct {
	On  []Entry `json:"on"`
	Off []Entry `json:"off"`
}

// Empty reports whether the schedule has no times (the display is left alone).
func (t Timers) Empty() bool { return len(t.On) == 0 && len(t.Off) == 0 }

var weekDays = map[string]time.Weekday{
	"SUN": time.Sunday, "MON": time.Monday, "TUE": time.Tuesday, "WED": time.Wednesday,
	"THU": time.Thursday, "FRI": time.Friday, "SAT": time.Saturday,
}

// ParseTimers parses the TIMERS setting of the Castmill device app. An empty
// string is an empty schedule.
func ParseTimers(s string) (Timers, error) {
	var t Timers
	if s == "" {
		return t, nil
	}
	if err := json.Unmarshal([]byte(s), &t); err != nil {
		return Timers{}, fmt.Errorf("timers: %w", err)
	}
	for _, list := range [][]Entry{t.On, t.Off} {
		for _, e := range list {
			if e.Hours < 0 || e.Hours > 23 || e.Minutes < 0 || e.Minutes > 59 {
				return Timers{}, fmt.Errorf("timers: invalid time %02d:%02d", e.Hours, e.Minutes)
			}
			for _, d := range e.WeekDays {
				if _, ok := weekDays[d]; !ok && d != "ALL" {
					return Timers{}, fmt.Errorf("timers: invalid week day %q", d)
				}
			}
		}
	}
	return t, nil
}

// last returns the most recent time at or before now that one of the entries
// fired, within the past week; zero if none.
func last(entries []Entry, now time.Time) time.Time {
	var best time.Time
	for _, e := range entries {
		all := false
		days := map[time.Weekday]bool{}
		for _, d := range e.WeekDays {
			if d == "ALL" {
				all = true
			} else {
				days[weekDays[d]] = true
			}
		}
		for back := 0; back <= 7; back++ {
			day := now.AddDate(0, 0, -back)
			if !all && !days[day.Weekday()] {
				continue
			}
			at := time.Date(day.Year(), day.Month(), day.Day(), e.Hours, e.Minutes, 0, 0, now.Location())
			if at.After(now) {
				continue
			}
			if at.After(best) {
				best = at
			}
			break // earlier days of this entry are older
		}
	}
	return best
}

// ShouldBeOff reports whether the schedule has the display off at now: the
// most recent event is an off time. An off and an on time in the same minute
// count as off, as in the device app's minute check. With no events in the
// past week, or an empty schedule, the display is on.
func (t Timers) ShouldBeOff(now time.Time) bool {
	lastOff := last(t.Off, now)
	if lastOff.IsZero() {
		return false
	}
	return !last(t.On, now).After(lastOff)
}
