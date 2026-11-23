package display

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Methods, in the order they are tried. HDMI-CEC puts TVs (and most signage
// displays) in standby and wakes them; DDC/CI does the same for monitors,
// which rarely have CEC. A display that supports neither stays on, showing
// the device app's black screen.
const (
	MethodCEC = "cec"
	MethodDDC = "ddc"
)

// ErrUnsupported means no method can control the display on an output.
var ErrUnsupported = errors.New("no display power control (HDMI-CEC or DDC/CI) on this output")

var validOutput = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// ValidOutput reports whether name looks like a DRM connector name.
func ValidOutput(name string) bool { return validOutput.MatchString(name) }

// Controller switches displays on and off. It remembers which method worked
// for each output and tries that one first.
type Controller struct {
	SysRoot string // "/" on a device
	DevRoot string // "/dev" on a device

	mu      sync.Mutex
	working map[string]string
}

// SetPower switches the display on output on or off and returns the method
// that did it.
func (c *Controller) SetPower(output string, on bool) (string, error) {
	if !validOutput.MatchString(output) {
		return "", fmt.Errorf("invalid output name %q", output)
	}
	c.mu.Lock()
	first := c.working[output]
	c.mu.Unlock()
	methods := []string{MethodCEC, MethodDDC}
	if first == MethodDDC {
		methods = []string{MethodDDC, MethodCEC}
	}
	var errs []string
	for _, m := range methods {
		var err error
		switch m {
		case MethodCEC:
			dev := c.cecDevice(output)
			if dev == "" {
				continue
			}
			err = cecPower(dev, on)
		case MethodDDC:
			dev := c.ddcDevice(output)
			if dev == "" {
				continue
			}
			err = ddcPower(dev, on)
		}
		if err == nil {
			c.mu.Lock()
			if c.working == nil {
				c.working = map[string]string{}
			}
			c.working[output] = m
			c.mu.Unlock()
			return m, nil
		}
		errs = append(errs, m+": "+err.Error())
	}
	if len(errs) == 0 {
		return "", ErrUnsupported
	}
	return "", errors.New(strings.Join(errs, "; "))
}

// connectorDir returns /sys/class/drm/cardN-<output>, or "".
func (c *Controller) connectorDir(output string) string {
	m, _ := filepath.Glob(filepath.Join(c.SysRoot, "sys/class/drm/card*-"+output))
	if len(m) == 0 {
		return ""
	}
	return m[0]
}

// ddcDevice returns the I2C device of the output's DDC bus, e.g. /dev/i2c-5.
// DRM links it as the connector's "ddc" attribute.
func (c *Controller) ddcDevice(output string) string {
	dir := c.connectorDir(output)
	if dir == "" {
		return ""
	}
	target, err := os.Readlink(filepath.Join(dir, "ddc"))
	if err != nil {
		return ""
	}
	name := filepath.Base(target)
	if !strings.HasPrefix(name, "i2c-") {
		return ""
	}
	dev := filepath.Join(c.DevRoot, name)
	if _, err := os.Stat(dev); err != nil {
		return "" // i2c-dev not loaded
	}
	return dev
}

// cecDevice returns the CEC adapter of the output, e.g. /dev/cec0. Adapters
// report the DRM connector they belong to (CEC_ADAP_G_CONNECTOR_INFO).
func (c *Controller) cecDevice(output string) string {
	dir := c.connectorDir(output)
	if dir == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(dir, "connector_id"))
	if err != nil {
		return ""
	}
	id, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 32)
	if err != nil {
		return ""
	}
	card, err := strconv.ParseUint(strings.TrimPrefix(strings.SplitN(filepath.Base(dir), "-", 2)[0], "card"), 10, 32)
	if err != nil {
		return ""
	}
	devs, _ := filepath.Glob(filepath.Join(c.DevRoot, "cec[0-9]*"))
	for _, dev := range devs {
		if gotCard, gotID, err := cecConnector(dev); err == nil && gotCard == uint32(card) && gotID == uint32(id) {
			return dev
		}
	}
	return ""
}

// ---- HDMI-CEC (linux/cec.h) ----------------------------------------------------

const (
	cecMsgImageViewOn = 0x04
	cecMsgStandby     = 0x36
	cecMsgActiveSrc   = 0x82
	cecTV             = 0x0
	cecBroadcast      = 0xf
	cecUnregistered   = 0xf
	cecPhysAddrNone   = 0xffff

	cecTxStatusOK = 0x01

	sizeofCECMsg      = 56 // struct cec_msg
	sizeofCECLogAddrs = 92 // struct cec_log_addrs
	sizeofCECConnInfo = 68 // struct cec_connector_info
)

// cecLogAddrs is struct cec_log_addrs claiming one Playback logical address,
// which a display power controller is to the TV.
func cecLogAddrs() []byte {
	b := make([]byte, sizeofCECLogAddrs)
	b[6] = 6                                       // cec_version: CEC_OP_CEC_VERSION_2_0
	b[7] = 1                                       // num_log_addrs
	binary.NativeEndian.PutUint32(b[8:], 0xffffff) // vendor_id: CEC_VENDOR_ID_NONE
	binary.NativeEndian.PutUint32(b[12:], 1)       // flags: CEC_LOG_ADDRS_FL_ALLOW_UNREG_FALLBACK
	copy(b[16:31], "Castmill OS")                  // osd_name
	b[31] = 4                                      // primary_device_type: PLAYBACK
	b[35] = 1                                      // log_addr_type: CEC_LOG_ADDR_TYPE_PLAYBACK
	b[39] = 0x10                                   // all_device_types: CEC_OP_ALL_DEVTYPE_PLAYBACK
	return b
}

// cecMsg is struct cec_msg carrying msg.
func cecMsg(msg ...byte) []byte {
	b := make([]byte, sizeofCECMsg)
	binary.NativeEndian.PutUint32(b[16:], uint32(len(msg))) // len
	copy(b[32:48], msg)
	return b
}

func cecTxStatus(b []byte) byte { return b[50] }

// cecMessages returns the messages that switch the TV on or off, sent from
// logical address la (cecUnregistered while the adapter has none, for
// example while a TV in standby holds the HDMI hotplug line low).
func cecMessages(on bool, la byte, physAddr uint16) [][]byte {
	if !on {
		return [][]byte{{la<<4 | cecTV, cecMsgStandby}}
	}
	msgs := [][]byte{{la<<4 | cecTV, cecMsgImageViewOn}}
	if la != cecUnregistered && physAddr != cecPhysAddrNone {
		// Switch the TV to this input.
		msgs = append(msgs, []byte{la<<4 | cecBroadcast, cecMsgActiveSrc, byte(physAddr >> 8), byte(physAddr)})
	}
	return msgs
}

// ---- DDC/CI (VESA MCCS) --------------------------------------------------------

const (
	ddcAddr       = 0x37
	vcpPowerMode  = 0xd6
	vcpPowerOn    = 0x01
	vcpPowerOffDP = 0x04 // DPM off: the monitor still answers DDC/CI, so it can be woken
)

// ddcSetVCP is the DDC/CI "Set VCP Feature" packet written to address 0x37.
func ddcSetVCP(code byte, value uint16) []byte {
	p := []byte{0x51, 0x84, 0x03, code, byte(value >> 8), byte(value)}
	chk := byte(ddcAddr << 1)
	for _, x := range p {
		chk ^= x
	}
	return append(p, chk)
}
