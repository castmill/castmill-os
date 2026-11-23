package telemetry

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// Hardware is the static hardware description shown in the Castmill
// dashboard's device details (with the GPU report from the kiosk shell).
type Hardware struct {
	CPU    string `json:"cpu,omitempty"`    // e.g. "Intel N100 (4 cores)"
	Memory string `json:"memory,omitempty"` // installed RAM, e.g. "8 GB"
}

// ReadHardware describes the CPU and memory of the system at root.
func ReadHardware(root string) Hardware {
	return Hardware{CPU: cpuModel(root), Memory: memorySize(root)}
}

// Arm CPU part numbers (implementer 0x41) of the cores in supported and
// likely hardware.
var armParts = map[string]string{
	"0xd03": "Cortex-A53", "0xd04": "Cortex-A35", "0xd05": "Cortex-A55",
	"0xd07": "Cortex-A57", "0xd08": "Cortex-A72", "0xd09": "Cortex-A73",
	"0xd0a": "Cortex-A75", "0xd0b": "Cortex-A76", "0xd0c": "Neoverse-N1",
	"0xd0d": "Cortex-A77", "0xd41": "Cortex-A78", "0xd44": "Cortex-X1",
	"0xd46": "Cortex-A510", "0xd47": "Cortex-A710", "0xd48": "Cortex-X2",
	"0xd4b": "Cortex-A78C", "0xd4d": "Cortex-A715", "0xd4e": "Cortex-X3",
	"0xd80": "Cortex-A520", "0xd81": "Cortex-A720", "0xd82": "Cortex-X4",
}

var armImplementers = map[string]string{"0x41": "Arm", "0x51": "Qualcomm", "0x61": "Apple"}

func cpuModel(root string) string {
	f, err := os.Open(filepath.Join(root, "proc/cpuinfo"))
	if err != nil {
		return ""
	}
	defer f.Close()
	var model, implementer, part string
	threads, cores := 0, 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "processor":
			threads++
		case "model name":
			if model == "" {
				model = v
			}
		case "cpu cores": // x86: physical cores per package
			if cores == 0 {
				fmt.Sscan(v, &cores)
			}
		case "CPU implementer":
			if implementer == "" {
				implementer = v
			}
		case "CPU part":
			if part == "" {
				part = v
			}
		}
	}
	if model == "" && implementer != "" {
		if implementer == "0x41" {
			model = armParts[part]
		}
		if model == "" {
			model = armImplementers[implementer]
		}
	}
	if model == "" {
		return ""
	}
	model = cleanCPUName(model)
	switch {
	case threads == 0:
		return model
	case cores > 0 && cores < threads:
		return fmt.Sprintf("%s (%d cores, %d threads)", model, cores, threads)
	case threads == 1:
		return model + " (1 core)"
	default:
		return fmt.Sprintf("%s (%d cores)", model, threads)
	}
}

// cleanCPUName drops trademark signs, the "CPU @ 2.40GHz" suffix and extra
// spaces: "Intel(R) Core(TM) i5-8259U CPU @ 2.30GHz" -> "Intel Core i5-8259U".
func cleanCPUName(s string) string {
	for _, tm := range []string{"(R)", "(r)", "(TM)", "(tm)"} {
		s = strings.ReplaceAll(s, tm, "")
	}
	if i := strings.Index(s, " @ "); i >= 0 {
		s = s[:i]
	}
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSuffix(s, " CPU")
	s = strings.TrimSuffix(s, " Processor")
	return s
}

// memorySize is the installed RAM: MemTotal (which excludes memory reserved
// by firmware and the kernel) rounded up to whole GB.
func memorySize(root string) string {
	f, err := os.Open(filepath.Join(root, "proc/meminfo"))
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var kb uint64
		if _, err := fmt.Sscanf(sc.Text(), "MemTotal: %d kB", &kb); err == nil && kb > 0 {
			gb := math.Ceil(float64(kb) / (1 << 20))
			return fmt.Sprintf("%.0f GB", gb)
		}
	}
	return ""
}
