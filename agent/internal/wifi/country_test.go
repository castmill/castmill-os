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
	if ValidCountry("se") || ValidCountry("SWE") || !ValidCountry("SE") {
		t.Error("ValidCountry")
	}
}
