// Package telemetry samples device health metrics from /proc and /sys
// (MGT-004). Data has the shape of Castmill's TelemetryData
// (packages/device/src/interfaces/machine.ts in the Castmill repository), so
// the kiosk shell hands it to the Castmill device app unchanged.
package telemetry

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Usage is a total/used pair in bytes.
type Usage struct {
	TotalBytes uint64 `json:"totalBytes"`
	UsedBytes  uint64 `json:"usedBytes"`
}

// LoadAvg holds the 1, 5 and 15 minute load averages.
type LoadAvg struct {
	One     float64 `json:"one"`
	Five    float64 `json:"five"`
	Fifteen float64 `json:"fifteen"`
}

// Temperature is one sensor reading.
type Temperature struct {
	Label   string  `json:"label"`
	Celsius float64 `json:"celsius"`
}

// Network describes the interface carrying the default route.
type Network struct {
	Type                      string `json:"type,omitempty"` // ethernet / wifi
	WifiSignalStrengthPercent *int   `json:"wifiSignalStrengthPercent,omitempty"`
	IPAddress                 string `json:"ipAddress,omitempty"`
}

// Data mirrors Castmill's TelemetryData. Fields the platform can't measure
// are omitted.
type Data struct {
	Storage         *Usage        `json:"storage,omitempty"` // the data partition
	Memory          *Usage        `json:"memory,omitempty"`
	CPUUsagePercent *float64      `json:"cpuUsagePercent,omitempty"`
	CPULoadAvg      *LoadAvg      `json:"cpuLoadAvg,omitempty"`
	Temperatures    []Temperature `json:"temperatures,omitempty"`
	Network         *Network      `json:"network,omitempty"`
	UptimeSeconds   float64       `json:"uptimeSeconds,omitempty"`
}

// Sampler keeps the previous CPU counters to compute utilisation.
type Sampler struct {
	Root     string // filesystem root, "/" on the device (tests use a fixture)
	DataPath string // mount point of the data partition
	prevIdle uint64
	prevAll  uint64
}

func (s *Sampler) p(rel string) string { return filepath.Join(s.Root, rel) }

// Sample collects one record. The first call can't report CPU usage (it
// needs two samples); later calls report usage since the previous call.
func (s *Sampler) Sample() *Data {
	d := &Data{UptimeSeconds: Uptime(s.Root)}
	if cpu, ok := s.cpu(); ok {
		d.CPUUsagePercent = &cpu
	}
	d.CPULoadAvg = s.load()
	if total, avail := s.mem(); total > 0 {
		d.Memory = &Usage{TotalBytes: total * 1024, UsedBytes: (total - avail) * 1024}
	}
	if total, free := diskUsage(s.DataPath); total > 0 {
		d.Storage = &Usage{TotalBytes: total, UsedBytes: total - free}
	}
	d.Temperatures = s.temps()
	d.Network = s.network()
	return d
}

func (s *Sampler) cpu() (float64, bool) {
	f, err := os.Open(s.p("proc/stat"))
	if err != nil {
		return 0, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0, false
	}
	fields := strings.Fields(sc.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, false
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
		return 0, false
	}
	return float64(dAll-dIdle) * 100 / float64(dAll), true
}

func (s *Sampler) load() *LoadAvg {
	b, err := os.ReadFile(s.p("proc/loadavg"))
	if err != nil {
		return nil
	}
	var l LoadAvg
	if _, err := fmt.Sscan(string(b), &l.One, &l.Five, &l.Fifteen); err != nil {
		return nil
	}
	return &l
}

func (s *Sampler) mem() (totalKB, availKB uint64) {
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
			totalKB = v
		case "MemAvailable:":
			availKB = v
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

func (s *Sampler) temps() []Temperature {
	zones, _ := filepath.Glob(s.p("sys/class/thermal/thermal_zone*"))
	var out []Temperature
	for _, z := range zones {
		tb, err := os.ReadFile(filepath.Join(z, "temp"))
		if err != nil {
			continue
		}
		milli, err := strconv.ParseFloat(strings.TrimSpace(string(tb)), 64)
		if err != nil {
			continue
		}
		label := filepath.Base(z)
		if nb, err := os.ReadFile(filepath.Join(z, "type")); err == nil {
			label = strings.TrimSpace(string(nb))
		}
		out = append(out, Temperature{Label: label, Celsius: milli / 1000})
	}
	return out
}

// network reports the interface that carries the IPv4 default route.
func (s *Sampler) network() *Network {
	iface := defaultRouteInterface(s.Root)
	if iface == "" {
		return nil
	}
	n := &Network{Type: "ethernet"}
	if _, err := os.Stat(s.p("sys/class/net/" + iface + "/wireless")); err == nil {
		n.Type = "wifi"
		if q, ok := wifiQuality(s.Root, iface); ok {
			n.WifiSignalStrengthPercent = &q
		}
	}
	if s.Root == "/" {
		if ifi, err := net.InterfaceByName(iface); err == nil {
			if addrs, err := ifi.Addrs(); err == nil {
				for _, a := range addrs {
					if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
						n.IPAddress = ipn.IP.String()
						break
					}
				}
			}
		}
	}
	return n
}

func defaultRouteInterface(root string) string {
	f, err := os.Open(filepath.Join(root, "proc/net/route"))
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) > 2 && fs[1] == "00000000" && fs[0] != "Iface" {
			return fs[0]
		}
	}
	return ""
}

// wifiQuality converts /proc/net/wireless link quality (0-70) to percent.
func wifiQuality(root, iface string) (int, bool) {
	f, err := os.Open(filepath.Join(root, "proc/net/wireless"))
	if err != nil {
		return 0, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) > 2 && strings.TrimSuffix(fs[0], ":") == iface {
			q, err := strconv.ParseFloat(strings.TrimSuffix(fs[2], "."), 64)
			if err != nil {
				return 0, false
			}
			p := int(q * 100 / 70)
			return min(max(p, 0), 100), true
		}
	}
	return 0, false
}

// ConnectedOutputs lists DRM connectors and whether they are connected,
// using the compositor's names (card0-HDMI-A-1 -> HDMI-A-1).
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
