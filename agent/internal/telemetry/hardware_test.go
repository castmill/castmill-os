package telemetry

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeProc(t *testing.T, cpuinfo, meminfo string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "proc"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]string{"cpuinfo": cpuinfo, "meminfo": meminfo} {
		if err := os.WriteFile(filepath.Join(root, "proc", name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func x86CPU(n, cores int, name string) string {
	var b strings.Builder
	for i := range n {
		b.WriteString("processor\t: " + strconv.Itoa(i) + "\n")
		b.WriteString("model name\t: " + name + "\n")
		b.WriteString("cpu cores\t: " + strconv.Itoa(cores) + "\n\n")
	}
	return b.String()
}

func TestReadHardware(t *testing.T) {
	pi5 := strings.Repeat("processor\t: 0\nBogoMIPS\t: 108.00\nCPU implementer\t: 0x41\nCPU part\t: 0xd0b\n\n", 4)
	apple := "processor\t: 0\nCPU implementer\t: 0x61\nCPU part\t: 0x039\n\n"
	tests := []struct {
		name, cpuinfo, meminfo string
		want                   Hardware
	}{
		{"N100", x86CPU(4, 4, "Intel(R) N100"), "MemTotal:        7880000 kB\n",
			Hardware{CPU: "Intel N100 (4 cores)", Memory: "8 GB"}},
		{"hyper-threading", x86CPU(4, 2, "Intel(R) Core(TM) i5-8259U CPU @ 2.30GHz"), "MemTotal: 16100000 kB\n",
			Hardware{CPU: "Intel Core i5-8259U (2 cores, 4 threads)", Memory: "16 GB"}},
		{"Raspberry Pi 5", pi5, "MemTotal:        4045000 kB\n",
			Hardware{CPU: "Cortex-A76 (4 cores)", Memory: "4 GB"}},
		{"Apple VM", apple, "", Hardware{CPU: "Apple (1 core)"}},
		{"nothing", "", "", Hardware{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReadHardware(writeProc(t, tt.cpuinfo, tt.meminfo)); got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
	if got := ReadHardware(t.TempDir()); got != (Hardware{}) {
		t.Errorf("no /proc: %+v", got)
	}
}
