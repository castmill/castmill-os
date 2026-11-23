package display

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.UTC)
	if err != nil {
		panic(err)
	}
	return t
}

func TestShouldBeOff(t *testing.T) {
	// Weekdays 08:00-20:00; off all weekend. 2026-11-23 is a Monday.
	timers, err := ParseTimers(`{"on":[{"hours":8,"minutes":0,"weekDays":["MON","TUE","WED","THU","FRI"]}],
		"off":[{"hours":20,"minutes":0,"weekDays":["ALL"]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		now string
		off bool
	}{
		{"2026-11-23 07:59", true},  // Monday before opening: Sunday 20:00 was last
		{"2026-11-23 08:00", false}, // on time, inclusive
		{"2026-11-23 19:59", false},
		{"2026-11-23 20:00", true},
		{"2026-11-28 12:00", true}, // Saturday: no on time since Friday 20:00
		{"2026-11-30 08:30", false},
	}
	for _, c := range cases {
		if got := timers.ShouldBeOff(at(c.now)); got != c.off {
			t.Errorf("%s: off=%v, want %v", c.now, got, c.off)
		}
	}
}

func TestShouldBeOffEdgeCases(t *testing.T) {
	if (Timers{}).ShouldBeOff(at("2026-11-23 03:00")) {
		t.Error("empty schedule: display should stay on")
	}
	onlyOn := Timers{On: []Entry{{Hours: 9, WeekDays: []string{"ALL"}}}}
	if onlyOn.ShouldBeOff(at("2026-11-23 03:00")) {
		t.Error("on times only: display should stay on")
	}
	onlyOff := Timers{Off: []Entry{{Hours: 22, WeekDays: []string{"FRI"}}}}
	if !onlyOff.ShouldBeOff(at("2026-11-23 03:00")) {
		t.Error("off time only, last Friday: display should be off")
	}
	same := Timers{On: []Entry{{Hours: 9, WeekDays: []string{"ALL"}}}, Off: []Entry{{Hours: 9, WeekDays: []string{"ALL"}}}}
	if !same.ShouldBeOff(at("2026-11-23 09:30")) {
		t.Error("on and off in the same minute: off wins")
	}
}

func TestParseTimersRejectsBadInput(t *testing.T) {
	for _, s := range []string{
		`not json`,
		`{"on":[{"hours":24,"minutes":0,"weekDays":["ALL"]}],"off":[]}`,
		`{"on":[],"off":[{"hours":1,"minutes":60,"weekDays":["ALL"]}]}`,
		`{"on":[{"hours":1,"minutes":0,"weekDays":["MONDAY"]}],"off":[]}`,
	} {
		if _, err := ParseTimers(s); err == nil {
			t.Errorf("ParseTimers(%s): no error", s)
		}
	}
	if tm, err := ParseTimers(""); err != nil || !tm.Empty() {
		t.Errorf("ParseTimers(\"\") = %v, %v", tm, err)
	}
}

func TestCECEncoding(t *testing.T) {
	m := cecMsg(0x40, cecMsgStandby)
	if len(m) != 56 || m[16] != 2 || m[32] != 0x40 || m[33] != 0x36 {
		t.Errorf("cec_msg = % x", m)
	}
	la := cecLogAddrs()
	if len(la) != 92 || la[7] != 1 || la[35] != 1 || string(la[16:27]) != "Castmill OS" {
		t.Errorf("cec_log_addrs = % x", la)
	}

	off := cecMessages(false, 4, 0x1000)
	if len(off) != 1 || !bytes.Equal(off[0], []byte{0x40, 0x36}) {
		t.Errorf("off = % x", off)
	}
	on := cecMessages(true, 4, 0x1000)
	if len(on) != 2 || !bytes.Equal(on[0], []byte{0x40, 0x04}) || !bytes.Equal(on[1], []byte{0x4f, 0x82, 0x10, 0x00}) {
		t.Errorf("on = % x", on)
	}
	// Unconfigured adapter (TV in standby with hotplug low): Image View On only.
	wake := cecMessages(true, cecUnregistered, cecPhysAddrNone)
	if len(wake) != 1 || !bytes.Equal(wake[0], []byte{0xf0, 0x04}) {
		t.Errorf("wake = % x", wake)
	}
}

func TestDDCPacket(t *testing.T) {
	if got, want := ddcSetVCP(vcpPowerMode, vcpPowerOn), []byte{0x51, 0x84, 0x03, 0xd6, 0x00, 0x01, 0x6f}; !bytes.Equal(got, want) {
		t.Errorf("set power on = % x, want % x", got, want)
	}
}

func TestDDCDevice(t *testing.T) {
	root := t.TempDir()
	conn := filepath.Join(root, "sys/class/drm/card1-HDMI-A-1")
	must(t, os.MkdirAll(conn, 0o755))
	must(t, os.Symlink("../../../devices/platform/i2c-7", filepath.Join(conn, "ddc")))
	dev := filepath.Join(root, "dev")
	must(t, os.MkdirAll(dev, 0o755))
	c := &Controller{SysRoot: root, DevRoot: dev}

	if got := c.ddcDevice("HDMI-A-1"); got != "" {
		t.Errorf("without i2c-dev: %q", got)
	}
	must(t, os.WriteFile(filepath.Join(dev, "i2c-7"), nil, 0o600))
	if got := c.ddcDevice("HDMI-A-1"); got != filepath.Join(dev, "i2c-7") {
		t.Errorf("ddcDevice = %q", got)
	}
	if got := c.ddcDevice("DP-1"); got != "" {
		t.Errorf("unknown output: %q", got)
	}
}

func TestSetPowerWithoutControl(t *testing.T) {
	root := t.TempDir()
	c := &Controller{SysRoot: root, DevRoot: filepath.Join(root, "dev")}
	if _, err := c.SetPower("Virtual-1", false); err != ErrUnsupported {
		t.Errorf("err = %v, want ErrUnsupported", err)
	}
	if _, err := c.SetPower("../../etc", false); err == nil || err == ErrUnsupported {
		t.Errorf("invalid name accepted: %v", err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
