package maintenance

import (
	"testing"
	"time"

	"github.com/castmill/castmill-os/agent/internal/api"
)

func at(day time.Weekday, hh, mm int) time.Time {
	// 2026-11-22 is a Sunday.
	base := time.Date(2026, 11, 22, hh, mm, 0, 0, time.UTC)
	return base.AddDate(0, 0, int(day))
}

func TestIn(t *testing.T) {
	tests := []struct {
		name string
		ws   []api.Window
		t    time.Time
		want bool
	}{
		{"default inside", nil, at(time.Monday, 3, 30), true},
		{"default before", nil, at(time.Monday, 2, 59), false},
		{"default end exclusive", nil, at(time.Monday, 5, 0), false},
		{"midnight cross late", []api.Window{{Start: "23:00", End: "02:00"}}, at(time.Tuesday, 23, 30), true},
		{"midnight cross early", []api.Window{{Start: "23:00", End: "02:00"}}, at(time.Wednesday, 1, 59), true},
		{"midnight cross outside", []api.Window{{Start: "23:00", End: "02:00"}}, at(time.Wednesday, 2, 0), false},
		{"day filter match", []api.Window{{Days: []string{"sun"}, Start: "10:00", End: "11:00"}}, at(time.Sunday, 10, 15), true},
		{"day filter miss", []api.Window{{Days: []string{"sun"}, Start: "10:00", End: "11:00"}}, at(time.Monday, 10, 15), false},
		// Saturday 23:00 -> Sunday 02:00: Sunday 01:00 belongs to Saturday's window.
		{"day filter cross midnight", []api.Window{{Days: []string{"sat"}, Start: "23:00", End: "02:00"}}, at(time.Sunday, 1, 0), true},
		{"day filter cross midnight wrong day", []api.Window{{Days: []string{"sat"}, Start: "23:00", End: "02:00"}}, at(time.Saturday, 1, 0), false},
		{"whole day", []api.Window{{Start: "00:00", End: "00:00"}}, at(time.Friday, 13, 0), true},
		{"invalid ignored", []api.Window{{Start: "25:00", End: "02:00"}}, at(time.Friday, 1, 0), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := In(tc.ws, tc.t); got != tc.want {
				t.Fatalf("In(%v, %v) = %v, want %v", tc.ws, tc.t, got, tc.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	if err := Validate([]api.Window{{Start: "03:00", End: "05:00", Days: []string{"Mon"}}}); err != nil {
		t.Fatal(err)
	}
	for _, w := range []api.Window{{Start: "3", End: "05:00"}, {Start: "03:00", End: "05:60"}, {Start: "01:00", End: "02:00", Days: []string{"monday"}}} {
		if Validate([]api.Window{w}) == nil {
			t.Fatalf("expected error for %+v", w)
		}
	}
}

func TestShouldActivate(t *testing.T) {
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	// 02:30 UTC = 03:30 in Madrid (CET, winter).
	now := time.Date(2026, 11, 23, 2, 30, 0, 0, time.UTC)
	p := Policy{Location: madrid, TimeTrusted: true}
	if d := ShouldActivate(p, now); !d.Activate {
		t.Fatalf("expected activation inside local window: %+v", d)
	}
	p.Location = time.UTC
	if d := ShouldActivate(p, now); d.Activate {
		t.Fatalf("02:30 UTC is outside the UTC window: %+v", d)
	}
	p.TimeTrusted = false
	p.Location = madrid
	if d := ShouldActivate(p, now); d.Activate {
		t.Fatal("must not activate with untrusted time")
	}
	p.ActivatePolicy = api.ActivateNow
	if d := ShouldActivate(p, now); !d.Activate {
		t.Fatal("apply-now must win even with untrusted time")
	}
	noon := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	p = Policy{Location: time.UTC, TimeTrusted: true, ActivatePolicy: api.ActivateSecurity, SecurityPolicy: "24h",
		InstalledAt: noon.Add(-25 * time.Hour)}
	if d := ShouldActivate(p, noon); !d.Activate {
		t.Fatalf("security release past deadline should activate: %+v", d)
	}
	p.InstalledAt = noon.Add(-time.Hour)
	if d := ShouldActivate(p, noon); d.Activate {
		t.Fatal("security release within deadline waits for the window")
	}
}
