// Package wifi sets up Wi-Fi on site through NetworkManager's D-Bus API: the
// networks in range, joining one, and whether the device is online. Used for
// the on-screen Wi-Fi setup (the kiosk shell's status page and the player
// menu) and for castmill.conf on a USB stick, all only while the device is
// offline.
//
// D-Bus instead of nmcli keeps the Wi-Fi password off command lines, where
// other local processes could read it.
package wifi

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/godbus/dbus/v5"
)

// ConnectionID is the NetworkManager connection Castmill OS manages; the one
// castmill.conf creates has the same id.
const ConnectionID = "castmill-wifi"

// Network is a Wi-Fi network in range.
type Network struct {
	SSID   string `json:"ssid"`
	Signal int    `json:"signal"` // 0-100
	Secure bool   `json:"secure"`
	Active bool   `json:"active"` // the device is connected to it
}

// Client is the Wi-Fi side of NetworkManager.
type Client interface {
	// Online reports whether NetworkManager sees full internet connectivity.
	Online(ctx context.Context) (bool, error)
	// Scan lists the networks in range, strongest first.
	Scan(ctx context.Context) ([]Network, error)
	// Current is the network the device is connected to (nil when none),
	// without scanning. ErrNoWiFi when the device has no Wi-Fi.
	Current(ctx context.Context) (*Network, error)
	// Join connects to a network and keeps it as castmill-wifi, replacing
	// the previous one only once the new one works.
	Join(ctx context.Context, ssid, psk string, hidden bool) error
}

// ErrNoWiFi: the device has no Wi-Fi adapter.
var ErrNoWiFi = errors.New("this device has no Wi-Fi")

var hexKey = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// Validate checks an SSID and a WPA passphrase (empty for open networks).
func Validate(ssid, psk string) error {
	if len(ssid) == 0 || len(ssid) > 32 {
		return errors.New("the network name must be 1 to 32 bytes")
	}
	if psk != "" && (len(psk) < 8 || len(psk) > 63) && !hexKey.MatchString(psk) {
		return errors.New("the password must be 8 to 63 characters")
	}
	return nil
}

// ---- NetworkManager over D-Bus ---------------------------------------------

const (
	nmService     = "org.freedesktop.NetworkManager"
	nmPath        = "/org/freedesktop/NetworkManager"
	nmIface       = "org.freedesktop.NetworkManager"
	deviceIface   = "org.freedesktop.NetworkManager.Device"
	wirelessIface = "org.freedesktop.NetworkManager.Device.Wireless"
	apIface       = "org.freedesktop.NetworkManager.AccessPoint"
	settingsPath  = "/org/freedesktop/NetworkManager/Settings"
	settingsIface = "org.freedesktop.NetworkManager.Settings"
	connIface     = "org.freedesktop.NetworkManager.Settings.Connection"
	activeIface   = "org.freedesktop.NetworkManager.Connection.Active"

	deviceTypeWiFi        = 2
	connectivityFull      = 4
	activeStateActivated  = 2
	activeStateDeactivate = 4
	joinTimeout           = 25 * time.Second // player calls time out after 30 s
)

// NM is a Client backed by NetworkManager.
type NM struct{ conn *dbus.Conn }

// Connect opens the system bus.
func Connect() (*NM, error) {
	c, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	return &NM{conn: c}, nil
}

// Close closes the bus connection.
func (n *NM) Close() error { return n.conn.Close() }

func (n *NM) obj(path dbus.ObjectPath) dbus.BusObject { return n.conn.Object(nmService, path) }

// Online implements Client.
func (n *NM) Online(ctx context.Context) (bool, error) {
	v, err := n.obj(nmPath).GetProperty(nmIface + ".Connectivity")
	if err != nil {
		return false, err
	}
	c, _ := v.Value().(uint32)
	return c == connectivityFull, nil
}

func (n *NM) wifiDevice(ctx context.Context) (dbus.ObjectPath, error) {
	var devices []dbus.ObjectPath
	if err := n.obj(nmPath).CallWithContext(ctx, nmIface+".GetDevices", 0).Store(&devices); err != nil {
		return "", err
	}
	for _, d := range devices {
		v, err := n.obj(d).GetProperty(deviceIface + ".DeviceType")
		if err == nil {
			if t, _ := v.Value().(uint32); t == deviceTypeWiFi {
				return d, nil
			}
		}
	}
	return "", ErrNoWiFi
}

// Current implements Client.
func (n *NM) Current(ctx context.Context) (*Network, error) {
	dev, err := n.wifiDevice(ctx)
	if err != nil {
		return nil, err
	}
	ap, err := prop[dbus.ObjectPath](n.obj(dev), wirelessIface+".ActiveAccessPoint")
	if err != nil || ap == "" || ap == "/" {
		return nil, nil
	}
	o := n.obj(ap)
	ssid, err := prop[[]byte](o, apIface+".Ssid")
	if err != nil || len(ssid) == 0 {
		return nil, nil
	}
	strength, _ := prop[byte](o, apIface+".Strength")
	flags, _ := prop[uint32](o, apIface+".Flags")
	wpa, _ := prop[uint32](o, apIface+".WpaFlags")
	rsn, _ := prop[uint32](o, apIface+".RsnFlags")
	return &Network{SSID: string(ssid), Signal: int(strength),
		Secure: flags&1 != 0 || wpa != 0 || rsn != 0, Active: true}, nil
}

// Scan implements Client.
func (n *NM) Scan(ctx context.Context) ([]Network, error) {
	dev, err := n.wifiDevice(ctx)
	if err != nil {
		return nil, err
	}
	// A scan request fails while one is running; the last results are fine.
	_ = n.obj(dev).CallWithContext(ctx, wirelessIface+".RequestScan", 0, map[string]dbus.Variant{}).Err
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(3 * time.Second):
	}
	var aps []dbus.ObjectPath
	if err := n.obj(dev).CallWithContext(ctx, wirelessIface+".GetAllAccessPoints", 0).Store(&aps); err != nil {
		return nil, err
	}
	var active dbus.ObjectPath
	if v, err := n.obj(dev).GetProperty(wirelessIface + ".ActiveAccessPoint"); err == nil {
		active, _ = v.Value().(dbus.ObjectPath)
	}
	var list []accessPoint
	for _, ap := range aps {
		o := n.obj(ap)
		ssid, _ := prop[[]byte](o, apIface+".Ssid")
		strength, _ := prop[byte](o, apIface+".Strength")
		flags, _ := prop[uint32](o, apIface+".Flags")
		wpa, _ := prop[uint32](o, apIface+".WpaFlags")
		rsn, _ := prop[uint32](o, apIface+".RsnFlags")
		list = append(list, accessPoint{ssid: string(ssid), strength: int(strength),
			secure: flags&1 != 0 || wpa != 0 || rsn != 0, active: ap == active})
	}
	return networks(list), nil
}

func prop[T any](o dbus.BusObject, name string) (T, error) {
	var zero T
	v, err := o.GetProperty(name)
	if err != nil {
		return zero, err
	}
	t, ok := v.Value().(T)
	if !ok {
		return zero, fmt.Errorf("%s: unexpected type %T", name, v.Value())
	}
	return t, nil
}

type accessPoint struct {
	ssid     string
	strength int
	secure   bool
	active   bool
}

// networks merges access points of the same network (strongest signal) and
// sorts them: the connected one first, then by signal.
func networks(aps []accessPoint) []Network {
	byName := map[string]*Network{}
	for _, ap := range aps {
		if ap.ssid == "" {
			continue // hidden networks are joined by name
		}
		n := byName[ap.ssid]
		if n == nil {
			n = &Network{SSID: ap.ssid}
			byName[ap.ssid] = n
		}
		n.Signal = max(n.Signal, ap.strength)
		n.Secure = n.Secure || ap.secure
		n.Active = n.Active || ap.active
	}
	out := make([]Network, 0, len(byName))
	for _, n := range byName {
		out = append(out, *n)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Active != out[j].Active {
			return out[i].Active
		}
		if out[i].Signal != out[j].Signal {
			return out[i].Signal > out[j].Signal
		}
		return out[i].SSID < out[j].SSID
	})
	return out
}

// Settings for a castmill-wifi connection (NetworkManager's D-Bus format).
func connectionSettings(ssid, psk string, hidden bool) map[string]map[string]dbus.Variant {
	s := map[string]map[string]dbus.Variant{
		"connection": {
			"id":                   dbus.MakeVariant(ConnectionID),
			"type":                 dbus.MakeVariant("802-11-wireless"),
			"autoconnect":          dbus.MakeVariant(true),
			"autoconnect-priority": dbus.MakeVariant(int32(10)),
		},
		"802-11-wireless": {
			"ssid":   dbus.MakeVariant([]byte(ssid)),
			"mode":   dbus.MakeVariant("infrastructure"),
			"hidden": dbus.MakeVariant(hidden),
		},
		"ipv4": {"method": dbus.MakeVariant("auto")},
		"ipv6": {"method": dbus.MakeVariant("auto")},
	}
	if psk != "" {
		s["802-11-wireless-security"] = map[string]dbus.Variant{
			"key-mgmt": dbus.MakeVariant("wpa-psk"),
			"psk":      dbus.MakeVariant(psk),
		}
	}
	return s
}

// Join implements Client.
func (n *NM) Join(ctx context.Context, ssid, psk string, hidden bool) error {
	if err := Validate(ssid, psk); err != nil {
		return err
	}
	dev, err := n.wifiDevice(ctx)
	if err != nil {
		return err
	}
	old, err := n.connectionsWithID(ctx, ConnectionID)
	if err != nil {
		return err
	}
	var conn, active dbus.ObjectPath
	if err := n.obj(nmPath).CallWithContext(ctx, nmIface+".AddAndActivateConnection", 0,
		connectionSettings(ssid, psk, hidden), dev, dbus.ObjectPath("/")).Store(&conn, &active); err != nil {
		return fmt.Errorf("couldn't join %q: %w", ssid, err)
	}
	if err := n.waitActivated(ctx, active); err != nil {
		// Keep the previous network; drop the one that doesn't work.
		_ = n.obj(conn).CallWithContext(context.Background(), connIface+".Delete", 0).Err
		return fmt.Errorf("couldn't join %q (wrong password, or out of range?)", ssid)
	}
	for _, o := range old {
		if o != conn {
			_ = n.obj(o).CallWithContext(ctx, connIface+".Delete", 0).Err
		}
	}
	return nil
}

func (n *NM) waitActivated(ctx context.Context, active dbus.ObjectPath) error {
	ctx, cancel := context.WithTimeout(ctx, joinTimeout)
	defer cancel()
	for {
		state, err := prop[uint32](n.obj(active), activeIface+".State")
		switch {
		case err != nil:
			return err // the active connection is gone: activation failed
		case state == activeStateActivated:
			return nil
		case state >= activeStateDeactivate:
			return errors.New("deactivated")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (n *NM) connectionsWithID(ctx context.Context, id string) ([]dbus.ObjectPath, error) {
	var conns []dbus.ObjectPath
	if err := n.obj(settingsPath).CallWithContext(ctx, settingsIface+".ListConnections", 0).Store(&conns); err != nil {
		return nil, err
	}
	var out []dbus.ObjectPath
	for _, c := range conns {
		var s map[string]map[string]dbus.Variant
		if err := n.obj(c).CallWithContext(ctx, connIface+".GetSettings", 0).Store(&s); err != nil {
			continue
		}
		if v, ok := s["connection"]["id"]; ok {
			if got, _ := v.Value().(string); got == id {
				out = append(out, c)
			}
		}
	}
	return out, nil
}

// ReloadConnections makes NetworkManager re-read its keyfiles (after
// castmill.conf from a USB stick wrote one).
func (n *NM) ReloadConnections(ctx context.Context) error {
	var ok bool
	return n.obj(settingsPath).CallWithContext(ctx, settingsIface+".ReloadConnections", 0).Store(&ok)
}
