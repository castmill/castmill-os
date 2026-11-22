// Package telemetry samples device health metrics from /proc and /sys
// (MGT-004).
package telemetry

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/castmill/castmill-os/agent/internal/api"
)

// Sampler keeps the previous CPU counters to compute utilisation.
type Sampler struct {
	Root     string // filesystem root, "/" on the device (tests use a fixture)
	DataPath string // mount point of the data partition
	prevIdle uint64
	prevAll  uint64
}

func (s *Sampler) p(rel string) string { return filepath.Join(s.Root, rel) }

// Sample collects one telemetry record.
func (s *Sampler) Sample() *api.Telemetry {
	t := &api.Telemetry{Time: time.Now().UTC(), Temperatures: map[string]float64{}, StorageWear: map[string]string{}}
	t.CPUPercent = s.cpu()
	t.Load1 = s.load()
	t.MemTotalKB, t.MemAvailKB = s.mem()
	t.UptimeS = Uptime(s.Root)
	s.temps(t.Temperatures)
	if b, err := os.ReadFile(s.p("sys/devices/platform/soc/soc:firmware/get_throttled")); err == nil {
		t.Throttled = strings.TrimSpace(string(b))
	}
	t.DataTotalB, t.DataFreeB = diskUsage(s.DataPath)
	s.wear(t.StorageWear)
	return t
}

func (s *Sampler) cpu() float64 {
	f, err := os.Open(s.p("proc/stat"))
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0
	}
	fields := strings.Fields(sc.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0
	}
	var all, idle uint64
	for i, v := range fields[1:] {
		n, _ := strconv.ParseUint(v, 10, 64)
		all += n
		if i == 3 || i == 4 { // idle + iowait
			idle += n
		}
	}
	dAll, dIdle := all-s.prevAll, idle-s.prevIdle
	first := s.prevAll == 0
	s.prevAll, s.prevIdle = all, idle
	if first || dAll == 0 {
		return 0
	}
	return float64(dAll-dIdle) * 100 / float64(dAll)
}

func (s *Sampler) load() float64 {
	b, err := os.ReadFile(s.p("proc/loadavg"))
	if err != nil {
		return 0
	}
	var l float64
	fmt.Sscan(string(b), &l)
	return l
}

func (s *Sampler) mem() (total, avail uint64) {
	f, err := os.Open(s.p("proc/meminfo"))
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var k string
		var v uint64
		if _, err := fmt.Sscanf(sc.Text(), "%s %d", &k, &v); err != nil {
			continue
		}
		switch k {
		case "MemTotal:":
			total = v
		case "MemAvailable:":
			avail = v
		}
	}
	return
}

// Uptime returns seconds since boot.
func Uptime(root string) float64 {
	b, err := os.ReadFile(filepath.Join(root, "proc/uptime"))
	if err != nil {
		return 0
	}
	var u float64
	fmt.Sscan(string(b), &u)
	return u
}

func (s *Sampler) temps(out map[string]float64) {
	zones, _ := filepath.Glob(s.p("sys/class/thermal/thermal_zone*"))
	for _, z := range zones {
		tb, err := os.ReadFile(filepath.Join(z, "temp"))
		if err != nil {
			continue
		}
		milli, err := strconv.ParseFloat(strings.TrimSpace(string(tb)), 64)
		if err != nil {
			continue
		}
		name := filepath.Base(z)
		if nb, err := os.ReadFile(filepath.Join(z, "type")); err == nil {
			name = strings.TrimSpace(string(nb)) + "/" + name
		}
		out[name] = milli / 1000
	}
}

// wear reads eMMC life-time estimates (JEDEC, 0x01 = 0-10% used ... 0x0B =
// exceeded) and NVMe/SD where the kernel exposes them.
func (s *Sampler) wear(out map[string]string) {
	mmcs, _ := filepath.Glob(s.p("sys/class/mmc_host/mmc*/mmc*:*"))
	for _, m := range mmcs {
		if b, err := os.ReadFile(filepath.Join(m, "life_time")); err == nil {
			out[filepath.Base(m)+"/life_time"] = strings.TrimSpace(string(b))
		}
		if b, err := os.ReadFile(filepath.Join(m, "pre_eol_info")); err == nil {
			out[filepath.Base(m)+"/pre_eol_info"] = strings.TrimSpace(string(b))
		}
	}
	// NVMe wear needs SMART log pages (ioctl); added with the hardware lab.
}

// ConnectedOutputs lists DRM connectors whose status is "connected", using
// the compositor's names (card0-HDMI-A-1 -> HDMI-A-1).
func ConnectedOutputs(root string) map[string]bool {
	out := map[string]bool{}
	conns, _ := filepath.Glob(filepath.Join(root, "sys/class/drm/card*-*"))
	for _, c := range conns {
		b, err := os.ReadFile(filepath.Join(c, "status"))
		if err != nil {
			continue
		}
		base := filepath.Base(c)
		name := base[strings.Index(base, "-")+1:]
		out[name] = strings.TrimSpace(string(b)) == "connected"
	}
	return out
}
