package provision

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castmill/castmill-os/agent/internal/config"
)

func setup(t *testing.T, conf string) (Paths, string) {
	t.Helper()
	root := t.TempDir()
	boot := filepath.Join(root, "boot")
	os.MkdirAll(boot, 0o755)
	os.MkdirAll(filepath.Join(root, "zoneinfo/Europe"), 0o755)
	os.WriteFile(filepath.Join(root, "zoneinfo/Europe/Madrid"), []byte("TZif"), 0o644)
	os.WriteFile(filepath.Join(boot, "proxy-ca.pem"), []byte("-----BEGIN CERTIFICATE-----\n"), 0o644)
	src := filepath.Join(boot, "castmill.conf")
	if err := os.WriteFile(src, []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(root, "data/castmill/config")
	return Paths{
		Sources: []string{src}, NMDir: filepath.Join(cfg, "network"), ConfigDir: cfg,
		TimesyncConf: filepath.Join(cfg, "timesyncd.conf"), ProxyEnv: filepath.Join(cfg, "proxy.env"),
		Localtime: filepath.Join(cfg, "agent/localtime"), ZoneinfoDir: filepath.Join(root, "zoneinfo"),
		AgentUID: -1, AgentGID: -1,
	}, src
}

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

const full = `
[enrollment]
org_token = "tok-123"
group = "stores/madrid"
name_template = "{group}-{serial}"

[backend]
url = "https://dev.example:8443"

[network.wifi]
ssid = "Store WiFi"
psk = "s3cret pass"

[network.proxy]
https = "http://proxy.example:3128"
ca_cert_file = "proxy-ca.pem"

[time]
ntp = ["ntp.example.com", "pool.ntp.org"]
timezone = "Europe/Madrid"
`

func TestApplyFull(t *testing.T) {
	p, src := setup(t, full)
	res, err := Apply(p, false, quiet)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(res.Applied, ","); got != "wifi,proxy,ntp,timezone" {
		t.Fatalf("applied = %s", got)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("castmill.conf not removed from the boot partition")
	}
	prov, err := config.LoadProvisioning(filepath.Join(p.ConfigDir, "provisioning.json"))
	if err != nil {
		t.Fatal(err)
	}
	if prov.OrgToken != "tok-123" || prov.Group != "stores/madrid" || prov.BackendURL != "" {
		t.Fatalf("provisioning = %+v (backend override must be ignored on production)", prov)
	}
	nm, err := os.ReadFile(filepath.Join(p.NMDir, "castmill-wifi.nmconnection"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(nm), "ssid=Store WiFi\n") || !strings.Contains(string(nm), "psk=s3cret pass\n") {
		t.Fatalf("keyfile:\n%s", nm)
	}
	if fi, _ := os.Stat(filepath.Join(p.NMDir, "castmill-wifi.nmconnection")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("keyfile mode %v; NetworkManager ignores keyfiles readable by others", fi.Mode())
	}
	env, _ := os.ReadFile(p.ProxyEnv)
	if !strings.Contains(string(env), "https_proxy=http://proxy.example:3128") || !strings.Contains(string(env), "CASTMILL_PROXY_CA=") {
		t.Fatalf("proxy.env:\n%s", env)
	}
	ts, _ := os.ReadFile(p.TimesyncConf)
	if !strings.Contains(string(ts), "NTP=ntp.example.com pool.ntp.org") {
		t.Fatalf("timesyncd:\n%s", ts)
	}
	if l, err := os.Readlink(p.Localtime); err != nil || !strings.HasSuffix(l, "zoneinfo/Europe/Madrid") {
		t.Fatalf("localtime -> %q (%v)", l, err)
	}
}

func TestBackendOverrideOnDevImage(t *testing.T) {
	p, _ := setup(t, full)
	if _, err := Apply(p, true, quiet); err != nil {
		t.Fatal(err)
	}
	prov, _ := config.LoadProvisioning(filepath.Join(p.ConfigDir, "provisioning.json"))
	if prov.BackendURL != "https://dev.example:8443" {
		t.Fatalf("backend override not applied: %+v", prov)
	}
}

func TestRejectsTypos(t *testing.T) {
	p, src := setup(t, "[enrolment]\norg_token = \"x\"\n")
	if _, err := Apply(p, false, quiet); err == nil {
		t.Fatal("unknown section must be rejected")
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("an invalid file must be kept for the operator to fix")
	}
}

func TestNoFile(t *testing.T) {
	p, src := setup(t, "")
	os.Remove(src)
	res, err := Apply(p, false, quiet)
	if err != nil || res != nil {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestTimezoneValidation(t *testing.T) {
	dir := t.TempDir()
	for _, tz := range []string{"../etc/passwd", "/etc/passwd", "Mars/Olympus"} {
		if err := SetTimezone(dir, filepath.Join(dir, "lt"), tz); err == nil {
			t.Errorf("accepted %q", tz)
		}
	}
}

func TestApplyKeepsUSBFile(t *testing.T) {
	p, src := setup(t, full)
	p.Keep = true
	if _, err := Apply(p, false, quiet); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("castmill.conf removed from the USB stick:", err)
	}
	if _, err := os.Stat(filepath.Join(p.NMDir, "castmill-wifi.nmconnection")); err != nil {
		t.Fatal("Wi-Fi not applied:", err)
	}
}

func TestWiFiCountry(t *testing.T) {
	zoneTab := func(p Paths) {
		os.WriteFile(filepath.Join(p.ZoneinfoDir, "zone.tab"),
			[]byte("ES\t+4024-00341\tEurope/Madrid\tSpain (mainland)\n"), 0o644)
	}
	modprobe := func(p Paths) string {
		b, _ := os.ReadFile(filepath.Join(p.ConfigDir, "wifi-country.conf"))
		return string(b)
	}

	// From the time zone.
	p, _ := setup(t, "[time]\ntimezone = \"Europe/Madrid\"\n")
	zoneTab(p)
	if _, err := Apply(p, false, quiet); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(p.ConfigDir, "wifi-country.conf")
	if cc := ApplyWiFiCountry(p, conf, quiet); cc != "ES" ||
		!strings.Contains(modprobe(p), "ieee80211_regdom=ES\n") {
		t.Fatalf("country %q, modprobe.d %q", cc, modprobe(p))
	}

	// castmill.conf wins, in any case.
	p, _ = setup(t, "[network]\nwifi_country = \"se\"\n\n[time]\ntimezone = \"Europe/Madrid\"\n")
	zoneTab(p)
	if _, err := Apply(p, false, quiet); err != nil {
		t.Fatal(err)
	}
	if cc := ApplyWiFiCountry(p, filepath.Join(p.ConfigDir, "wifi-country.conf"), quiet); cc != "SE" {
		t.Fatalf("country %q", cc)
	}

	// No country (UTC): no module option.
	p, _ = setup(t, "")
	os.MkdirAll(p.ConfigDir, 0o755)
	if cc := ApplyWiFiCountry(p, filepath.Join(p.ConfigDir, "wifi-country.conf"), quiet); cc != "" ||
		strings.Contains(modprobe(p), "options") {
		t.Fatalf("country %q, modprobe.d %q", cc, modprobe(p))
	}

	// Not a country code, or not an assigned one.
	for _, cc := range []string{"SWE", "ZZ"} {
		p, _ = setup(t, "[network]\nwifi_country = \""+cc+"\"\n")
		if _, err := Apply(p, false, quiet); err == nil || !strings.Contains(err.Error(), "wifi_country") {
			t.Fatalf("%s: err = %v", cc, err)
		}
	}

	// A time zone alias (US/Eastern links to America/New_York).
	p, _ = setup(t, "[time]\ntimezone = \"US/Eastern\"\n")
	os.MkdirAll(filepath.Join(p.ZoneinfoDir, "US"), 0o755)
	os.WriteFile(filepath.Join(p.ZoneinfoDir, "US/Eastern"), []byte("TZif"), 0o644)
	os.WriteFile(filepath.Join(p.ZoneinfoDir, "zone.tab"),
		[]byte("US\t+404251-0740023\tAmerica/New_York\tEastern (most areas)\n"), 0o644)
	os.WriteFile(filepath.Join(p.ZoneinfoDir, "tzdata.zi"), []byte("L America/New_York US/Eastern\n"), 0o644)
	if _, err := Apply(p, false, quiet); err != nil {
		t.Fatal(err)
	}
	if cc := ApplyWiFiCountry(p, filepath.Join(p.ConfigDir, "wifi-country.conf"), quiet); cc != "US" {
		t.Fatalf("country %q", cc)
	}
}

// A Wi-Fi stack that is already loaded gets the country, and goes back to
// the worldwide rules when the device no longer has one.
func TestWiFiCountryLoadedStack(t *testing.T) {
	var set []string
	loaded, setter := wifiLoaded, setWiFiCountry
	t.Cleanup(func() { wifiLoaded, setWiFiCountry = loaded, setter })
	wifiLoaded = func() bool { return true }
	setWiFiCountry = func(cc string) error { set = append(set, cc); return nil }

	p, _ := setup(t, "[time]\ntimezone = \"Europe/Madrid\"\n")
	os.WriteFile(filepath.Join(p.ZoneinfoDir, "zone.tab"),
		[]byte("ES\t+4024-00341\tEurope/Madrid\tSpain (mainland)\n"), 0o644)
	if _, err := Apply(p, false, quiet); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(p.ConfigDir, "wifi-country.conf")
	ApplyWiFiCountry(p, conf, quiet)

	// Re-provisioned to UTC: no country.
	os.MkdirAll(filepath.Join(p.ZoneinfoDir, "Etc"), 0o755)
	os.WriteFile(filepath.Join(p.ZoneinfoDir, "Etc/UTC"), []byte("TZif"), 0o644)
	if err := SetTimezone(p.ZoneinfoDir, p.Localtime, "Etc/UTC"); err != nil {
		t.Fatal(err)
	}
	if cc := ApplyWiFiCountry(p, conf, quiet); cc != "" {
		t.Fatalf("country %q", cc)
	}
	if strings.Join(set, ",") != "ES,00" {
		t.Fatalf("set %v, want ES then 00", set)
	}
}
