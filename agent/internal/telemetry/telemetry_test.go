package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSample(t *testing.T) {
	root := t.TempDir()
	write(t, root, "proc/stat", "cpu  100 0 100 700 100 0 0 0 0 0\n")
	write(t, root, "proc/loadavg", "0.42 0.30 0.20 1/123 4567\n")
	write(t, root, "proc/meminfo", "MemTotal:        4000000 kB\nMemFree:  100 kB\nMemAvailable:    3000000 kB\n")
	write(t, root, "proc/uptime", "1234.5 999.0\n")
	write(t, root, "sys/class/thermal/thermal_zone0/temp", "48500\n")
	write(t, root, "sys/class/thermal/thermal_zone0/type", "cpu-thermal\n")
	write(t, root, "proc/net/route", "Iface\tDestination\tGateway\nwlan0\t00000000\t0102A8C0\n")
	write(t, root, "sys/class/net/wlan0/wireless/.keep", "")
	write(t, root, "proc/net/wireless", "Inter-| sta-|   Quality\n face | tus | link level noise\n wlan0: 0000   56.  -54.  -256 0 0 0 0 0 0\n")

	s := &Sampler{Root: root, DataPath: root}
	if got := s.Sample(); got.CPUUsagePercent != nil {
		t.Fatalf("first sample must not report CPU usage, got %v", *got.CPUUsagePercent)
	}
	write(t, root, "proc/stat", "cpu  200 0 200 1300 200 0 0 0 0 0\n") // +200 busy, +700 idle/iowait
	d := s.Sample()
	if d.CPUUsagePercent == nil || *d.CPUUsagePercent < 22 || *d.CPUUsagePercent > 23 { // 200/900
		t.Errorf("cpu = %v", d.CPUUsagePercent)
	}
	if *d.CPULoadAvg != (LoadAvg{0.42, 0.30, 0.20}) {
		t.Errorf("load = %+v", d.CPULoadAvg)
	}
	if d.Memory.TotalBytes != 4000000*1024 || d.Memory.UsedBytes != 1000000*1024 {
		t.Errorf("memory = %+v", d.Memory)
	}
	if d.UptimeSeconds != 1234.5 || d.Storage == nil || d.Storage.TotalBytes == 0 {
		t.Errorf("uptime/storage = %v %+v", d.UptimeSeconds, d.Storage)
	}
	if len(d.Temperatures) != 1 || d.Temperatures[0] != (Temperature{"cpu-thermal", 48.5}) {
		t.Errorf("temps = %+v", d.Temperatures)
	}
	if d.Network == nil || d.Network.Type != "wifi" || d.Network.WifiSignalStrengthPercent == nil || *d.Network.WifiSignalStrengthPercent != 80 {
		t.Errorf("network = %+v", d.Network)
	}

	// JSON field names follow Castmill's TelemetryData.
	b, _ := json.Marshal(d)
	for _, k := range []string{`"storage":{"totalBytes"`, `"memory":{"totalBytes"`, `"cpuUsagePercent"`,
		`"cpuLoadAvg":{"one":0.42,"five":0.3,"fifteen":0.2}`, `"temperatures":[{"label":"cpu-thermal","celsius":48.5}]`,
		`"network":{"type":"wifi","wifiSignalStrengthPercent":80}`, `"uptimeSeconds":1234.5`} {
		if !strings.Contains(string(b), k) {
			t.Errorf("JSON lacks %s: %s", k, b)
		}
	}
}

func TestConnectedOutputs(t *testing.T) {
	root := t.TempDir()
	write(t, root, "sys/class/drm/card0-HDMI-A-1/status", "connected\n")
	write(t, root, "sys/class/drm/card0-HDMI-A-2/status", "disconnected\n")
	write(t, root, "sys/class/drm/card1-Virtual-1/status", "connected\n")
	got := ConnectedOutputs(root)
	if !got["HDMI-A-1"] || got["HDMI-A-2"] || !got["Virtual-1"] || len(got) != 3 {
		t.Fatalf("got %v", got)
	}
}
