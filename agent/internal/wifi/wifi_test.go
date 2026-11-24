package wifi

import (
	"reflect"
	"testing"
)

func TestValidate(t *testing.T) {
	ok := []struct{ ssid, psk string }{
		{"Venue", "password123"},
		{"Open café", ""},
		{"32-bytes-ssid-xxxxxxxxxxxxxxxxxx", "12345678"},
		{"Hex", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
	}
	for _, c := range ok {
		if err := Validate(c.ssid, c.psk); err != nil {
			t.Errorf("Validate(%q, %q) = %v", c.ssid, c.psk, err)
		}
	}
	bad := []struct{ ssid, psk string }{
		{"", "password123"},
		{"33-bytes-ssid-xxxxxxxxxxxxxxxxxxx", "password123"},
		{"Venue", "short"},
		{"Venue", string(make([]byte, 64))},
	}
	for _, c := range bad {
		if err := Validate(c.ssid, c.psk); err == nil {
			t.Errorf("Validate(%q, %q) accepted", c.ssid, c.psk)
		}
	}
}

func TestNetworks(t *testing.T) {
	got := networks([]accessPoint{
		{ssid: "Venue", strength: 40, secure: true},
		{ssid: "Venue", strength: 70, secure: true}, // second access point
		{ssid: "Guest", strength: 90},
		{ssid: "", strength: 99}, // hidden
		{ssid: "Office", strength: 20, secure: true, active: true},
	})
	want := []Network{
		{SSID: "Office", Signal: 20, Secure: true, Active: true},
		{SSID: "Guest", Signal: 90},
		{SSID: "Venue", Signal: 70, Secure: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("networks = %+v\nwant %+v", got, want)
	}
}

func TestConnectionSettings(t *testing.T) {
	s := connectionSettings("Venue", "password123", true)
	if s["connection"]["id"].Value() != ConnectionID ||
		string(s["802-11-wireless"]["ssid"].Value().([]byte)) != "Venue" ||
		s["802-11-wireless"]["hidden"].Value() != true ||
		s["802-11-wireless-security"]["psk"].Value() != "password123" {
		t.Fatalf("settings = %v", s)
	}
	if _, ok := connectionSettings("Open", "", false)["802-11-wireless-security"]; ok {
		t.Fatal("open network with security settings")
	}
}
