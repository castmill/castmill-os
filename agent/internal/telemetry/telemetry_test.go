package telemetry

import (
	"os"
	"path/filepath"
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
	write(t, root, "sys/devices/platform/soc/soc:firmware/get_throttled", "0x50000\n")
	write(t, root, "sys/class/mmc_host/mmc0/mmc0:0001/life_time", "0x01 0x02\n")

	s := &Sampler{Root: root, DataPath: root}
	if got := s.Sample(); got.CPUPercent != 0 {
		t.Fatalf("first sample must report 0 CPU, got %v", got.CPUPercent)
	}
	write(t, root, "proc/stat", "cpu  200 0 200 1300 200 0 0 0 0 0\n") // +200 busy, +700 idle/iowait
	tel := s.Sample()
	if tel.CPUPercent < 22 || tel.CPUPercent > 23 { // 200/900
		t.Errorf("cpu = %v", tel.CPUPercent)
	}
	if tel.Load1 != 0.42 || tel.MemTotalKB != 4000000 || tel.MemAvailKB != 3000000 || tel.UptimeS != 1234.5 {
		t.Errorf("unexpected values: %+v", tel)
	}
	if tel.Temperatures["cpu-thermal/thermal_zone0"] != 48.5 {
		t.Errorf("temps = %v", tel.Temperatures)
	}
	if tel.Throttled != "0x50000" {
		t.Errorf("throttled = %q", tel.Throttled)
	}
	if tel.StorageWear["mmc0:0001/life_time"] != "0x01 0x02" {
		t.Errorf("wear = %v", tel.StorageWear)
	}
	if tel.DataTotalB == 0 {
		t.Error("data size not reported")
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
