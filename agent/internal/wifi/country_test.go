package wifi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCountryForZone(t *testing.T) {
	tab := filepath.Join(t.TempDir(), "zone.tab")
	body := "# comment\tignored\tEurope/Stockholm\n" +
		"SE\t+5920+01803\tEurope/Stockholm\n" +
		"ES\t+4024-00341\tEurope/Madrid\tSpain (mainland)\n"
	if err := os.WriteFile(tab, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for zone, want := range map[string]string{
		"Europe/Stockholm": "SE",
		"Europe/Madrid":    "ES",
		"UTC":              "",
		"Etc/GMT+1":        "",
	} {
		if got := CountryForZone(tab, zone); got != want {
			t.Errorf("CountryForZone(%q) = %q, want %q", zone, got, want)
		}
	}
	// Aliases resolve through tzdata.zi, next to zone.tab.
	links := "L Europe/Stockholm Europe/Alias\nL Europe/Alias Europe/Alias2\nL Etc/UTC UTC\n"
	if err := os.WriteFile(filepath.Join(filepath.Dir(tab), "tzdata.zi"), []byte(links), 0o644); err != nil {
		t.Fatal(err)
	}
	for zone, want := range map[string]string{
		"Europe/Alias":  "SE",
		"Europe/Alias2": "SE",
		"UTC":           "",
		"Nowhere/Zone":  "",
	} {
		if got := CountryForZone(tab, zone); got != want {
			t.Errorf("CountryForZone(%q) = %q, want %q", zone, got, want)
		}
	}
	if got := CountryForZone(filepath.Join(t.TempDir(), "missing"), "Europe/Stockholm"); got != "" {
		t.Errorf("missing zone.tab: %q", got)
	}
}

func TestModprobeConf(t *testing.T) {
	if got := ModprobeConf("SE"); !strings.Contains(got, "options cfg80211 ieee80211_regdom=SE\n") {
		t.Errorf("ModprobeConf(SE) = %q", got)
	}
	if got := ModprobeConf(""); strings.Contains(got, "options") {
		t.Errorf("ModprobeConf() = %q", got)
	}
	for cc, want := range map[string]bool{"SE": true, "US": true, "se": false, "SWE": false, "ZZ": false, "00": false} {
		if ValidCountry(cc) != want {
			t.Errorf("ValidCountry(%q) = %v", cc, !want)
		}
	}
	if err := SetCountry("ZZ"); err == nil {
		t.Error("SetCountry(ZZ) accepted")
	}
}
