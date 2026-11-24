// Package provision applies a castmill.conf provisioning file (spec 08).
//
// Runs as root ("castmill-agent provision", castmill-provision.service) early
// at boot, after /data is mounted and before NetworkManager and the agent.
// Looks for castmill.conf on the boot partition and on USB sticks, applies it
// and moves it into /data, deleting the plain-text original.
package provision

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/castmill/castmill-os/agent/internal/config"
	"github.com/castmill/castmill-os/agent/internal/wifi"
)

// Paths used by Apply (injectable for tests).
type Paths struct {
	Sources      []string // candidate castmill.conf files, in priority order
	NMDir        string   // NetworkManager keyfile directory (/data/castmill/config/network)
	ConfigDir    string   // /data/castmill/config
	TimesyncConf string   // /data/castmill/config/timesyncd.conf (linked from /etc)
	ProxyEnv     string   // /data/castmill/config/proxy.env (EnvironmentFile of agent & player)
	Localtime    string   // /data/castmill/config/agent/localtime (target of /etc/localtime)
	ZoneinfoDir  string   // /usr/share/zoneinfo
	AgentUID     int      // owner of agent-readable files, -1 to skip chown
	AgentGID     int
	// Keep leaves castmill.conf where it is (a USB stick used for many
	// devices); otherwise it is deleted after it was applied.
	Keep bool
}

// DefaultSources: boot partition first, then any mounted USB stick.
func DefaultSources(bootDir string) []string {
	src := []string{filepath.Join(bootDir, "castmill.conf")}
	usb, _ := filepath.Glob("/run/media/*/castmill.conf")
	return append(src, usb...)
}

// Result describes what was applied.
type Result struct {
	Source  string
	Applied []string
}

// Apply finds the first castmill.conf and applies it. allowBackend controls
// whether [backend] is honoured (development images only).
func Apply(p Paths, allowBackend bool, log *slog.Logger) (*Result, error) {
	var src string
	for _, s := range p.Sources {
		if _, err := os.Stat(s); err == nil {
			src = s
			break
		}
	}
	if src == "" {
		return nil, nil
	}
	conf, err := config.ParseCastmillConf(src)
	if err != nil {
		// Keep the file so the operator can fix it; report on the fallback screen.
		return nil, err
	}
	res := &Result{Source: src}
	dir := filepath.Dir(src)

	prov, err := config.LoadProvisioning(filepath.Join(p.ConfigDir, "provisioning.json"))
	if err != nil {
		return nil, err
	}
	prov.OrgToken = conf.Enrollment.OrgToken
	prov.Group = conf.Enrollment.Group
	prov.NameTemplate = conf.Enrollment.NameTemplate
	if conf.Backend.URL != "" || conf.Backend.CACertFile != "" {
		if !allowBackend {
			log.Warn("castmill.conf [backend] ignored: only allowed on development images")
		} else {
			prov.BackendURL = conf.Backend.URL
			if conf.Backend.CACertFile != "" {
				b, err := os.ReadFile(resolve(dir, conf.Backend.CACertFile))
				if err != nil {
					return nil, fmt.Errorf("backend CA: %w", err)
				}
				prov.BackendCA = string(b)
			}
			res.Applied = append(res.Applied, "backend")
		}
	}

	if cc := conf.Network.WiFiCountry; cc != "" {
		prov.WiFiCountry = cc
		res.Applied = append(res.Applied, "wifi_country")
	}

	if w := conf.Network.WiFi; w != nil {
		if err := writeWiFi(p.NMDir, w.SSID, w.PSK, w.Hidden); err != nil {
			return nil, err
		}
		res.Applied = append(res.Applied, "wifi")
	}

	if px := conf.Network.Proxy; px != nil {
		var env strings.Builder
		for _, kv := range [][2]string{{"http_proxy", px.HTTP}, {"https_proxy", px.HTTPS}, {"no_proxy", px.NoProxy}} {
			if kv[1] != "" {
				fmt.Fprintf(&env, "%s=%s\n%s=%s\n", kv[0], kv[1], strings.ToUpper(kv[0]), kv[1])
			}
		}
		if px.CACertFile != "" {
			b, err := os.ReadFile(resolve(dir, px.CACertFile))
			if err != nil {
				return nil, fmt.Errorf("proxy CA: %w", err)
			}
			caPath := filepath.Join(p.ConfigDir, "proxy-ca.pem")
			if err := config.WriteFileAtomic(caPath, b, 0o644); err != nil {
				return nil, err
			}
			fmt.Fprintf(&env, "CASTMILL_PROXY_CA=%s\n", caPath)
		}
		if err := config.WriteFileAtomic(p.ProxyEnv, []byte(env.String()), 0o644); err != nil {
			return nil, err
		}
		res.Applied = append(res.Applied, "proxy")
	}

	if len(conf.Time.NTP) > 0 {
		body := "# Generated from castmill.conf\n[Time]\nNTP=" + strings.Join(conf.Time.NTP, " ") + "\n"
		if err := config.WriteFileAtomic(p.TimesyncConf, []byte(body), 0o644); err != nil {
			return nil, err
		}
		res.Applied = append(res.Applied, "ntp")
	}
	if tz := conf.Time.Timezone; tz != "" {
		if err := SetTimezone(p.ZoneinfoDir, p.Localtime, tz); err != nil {
			return nil, err
		}
		prov.Timezone = tz
		res.Applied = append(res.Applied, "timezone")
	}

	provPath := filepath.Join(p.ConfigDir, "provisioning.json")
	if err := config.WriteJSONAtomic(provPath, prov, 0o640); err != nil {
		return nil, err
	}
	if p.AgentUID >= 0 {
		_ = os.Chown(provPath, 0, p.AgentGID) // readable by the agent, contains the org token
	}

	// Keep a copy for support, without secrets in plain text on /boot.
	raw, _ := os.ReadFile(src)
	if err := config.WriteFileAtomic(filepath.Join(p.ConfigDir, "castmill.conf.applied"), raw, 0o600); err != nil {
		return nil, err
	}
	if p.Keep {
		return res, nil
	}
	if err := os.Remove(src); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Warn("could not delete provisioning file (read-only medium?)", "path", src, "err", err)
	}
	return res, nil
}

func resolve(dir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dir, p)
}

// writeWiFi creates a NetworkManager keyfile (root-owned, 0600 as NM requires).
func writeWiFi(nmDir, ssid, psk string, hidden bool) error {
	if strings.ContainsAny(ssid, "\n\r") || strings.ContainsAny(psk, "\n\r") {
		return errors.New("wifi ssid/psk must not contain newlines")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[connection]\nid=castmill-wifi\ntype=wifi\nautoconnect=true\nautoconnect-priority=10\n\n")
	fmt.Fprintf(&b, "[wifi]\nmode=infrastructure\nssid=%s\n", ssid)
	if hidden {
		b.WriteString("hidden=true\n")
	}
	if psk != "" {
		fmt.Fprintf(&b, "\n[wifi-security]\nkey-mgmt=wpa-psk\npsk=%s\n", psk)
	}
	b.WriteString("\n[ipv4]\nmethod=auto\n\n[ipv6]\nmethod=auto\naddr-gen-mode=stable-privacy\n")
	if err := os.MkdirAll(nmDir, 0o700); err != nil {
		return err
	}
	return config.WriteFileAtomic(filepath.Join(nmDir, "castmill-wifi.nmconnection"), []byte(b.String()), 0o600)
}

// SetTimezone points the localtime symlink at a zoneinfo file.
func SetTimezone(zoneinfoDir, localtime, tz string) error {
	if strings.Contains(tz, "..") || strings.HasPrefix(tz, "/") {
		return fmt.Errorf("invalid timezone %q", tz)
	}
	target := filepath.Join(zoneinfoDir, tz)
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("unknown timezone %q", tz)
	}
	if err := os.MkdirAll(filepath.Dir(localtime), 0o755); err != nil {
		return err
	}
	tmp := localtime + ".tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, localtime)
}

// WiFiCountry is the Wi-Fi country of the device: castmill.conf's
// wifi_country, else the country of its time zone (the localtime symlink),
// else "" (the kernel's worldwide rules).
func WiFiCountry(configDir, localtime, zoneTab string) string {
	if prov, err := config.LoadProvisioning(filepath.Join(configDir, "provisioning.json")); err == nil &&
		wifi.ValidCountry(prov.WiFiCountry) {
		return prov.WiFiCountry
	}
	target, err := os.Readlink(localtime)
	if err != nil {
		return ""
	}
	zone := target
	if i := strings.Index(target, "zoneinfo/"); i >= 0 {
		zone = target[i+len("zoneinfo/"):]
	}
	return wifi.CountryForZone(zoneTab, zone)
}

// The running Wi-Fi stack (replaced in tests).
var (
	wifiLoaded = func() bool {
		_, err := os.Stat("/sys/module/cfg80211")
		return err == nil
	}
	setWiFiCountry = wifi.SetCountry
)

// ApplyWiFiCountry writes the cfg80211 module option (for a Wi-Fi stack that
// loads later; /etc/modprobe.d links to it) and sets the country of one
// that's already loaded. Returns the country ("" for none).
func ApplyWiFiCountry(p Paths, modprobeConf string, log *slog.Logger) string {
	cc := WiFiCountry(p.ConfigDir, p.Localtime, filepath.Join(p.ZoneinfoDir, "zone.tab"))
	if err := config.WriteFileAtomic(modprobeConf, []byte(wifi.ModprobeConf(cc)), 0o644); err != nil {
		log.Warn("Wi-Fi country module option", "err", err)
	}
	if wifiLoaded() {
		// Without a country (a time zone like UTC), a loaded stack goes back
		// to the worldwide rules instead of keeping the previous country's.
		set := cc
		if set == "" {
			set = wifi.WorldCountry
		}
		if err := setWiFiCountry(set); err != nil {
			log.Warn("Wi-Fi country", "err", err)
		}
	}
	return cc
}
