// Package system wraps the OS facilities the agent needs: systemd and logind
// over D-Bus (authorised for the agent user by polkit rules shipped in the
// image), sd_notify, the clock and the factory-reset flag.
package system

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// Notify sends an sd_notify message (READY=1, WATCHDOG=1, STATUS=...). It is a
// no-op outside systemd.
func Notify(state string) error {
	sock := os.Getenv("NOTIFY_SOCKET")
	if sock == "" {
		return nil
	}
	if strings.HasPrefix(sock, "@") {
		sock = "\x00" + sock[1:]
	}
	c, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: sock, Net: "unixgram"})
	if err != nil {
		return err
	}
	defer c.Close()
	_, err = c.Write([]byte(state))
	return err
}

// WatchdogInterval returns half of WATCHDOG_USEC, or 0 when not enabled.
func WatchdogInterval() time.Duration {
	var usec int64
	if _, err := fmt.Sscan(os.Getenv("WATCHDOG_USEC"), &usec); err != nil || usec <= 0 {
		return 0
	}
	return time.Duration(usec) * time.Microsecond / 2
}

// Manager controls systemd units and power state.
type Manager interface {
	RestartUnit(ctx context.Context, unit string) error
	Reboot(ctx context.Context) error
	UnitActive(ctx context.Context, unit string) (string, error)
	FailedUnits(ctx context.Context) ([]string, error)
}

// DBusManager implements Manager on the system bus.
type DBusManager struct{ conn *dbus.Conn }

// NewManager connects to the system bus.
func NewManager() (*DBusManager, error) {
	c, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	return &DBusManager{conn: c}, nil
}

func (m *DBusManager) systemd() dbus.BusObject {
	return m.conn.Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1")
}

// RestartUnit restarts a unit (polkit restricts the agent to castmill-* units).
func (m *DBusManager) RestartUnit(ctx context.Context, unit string) error {
	var job dbus.ObjectPath
	return m.systemd().CallWithContext(ctx, "org.freedesktop.systemd1.Manager.RestartUnit", 0, unit, "replace").Store(&job)
}

// Reboot asks logind for a reboot.
func (m *DBusManager) Reboot(ctx context.Context) error {
	return m.conn.Object("org.freedesktop.login1", "/org/freedesktop/login1").
		CallWithContext(ctx, "org.freedesktop.login1.Manager.Reboot", 0, false).Err
}

// UnitActive returns ActiveState of a unit ("active", "failed", ...).
func (m *DBusManager) UnitActive(ctx context.Context, unit string) (string, error) {
	var path dbus.ObjectPath
	if err := m.systemd().CallWithContext(ctx, "org.freedesktop.systemd1.Manager.LoadUnit", 0, unit).Store(&path); err != nil {
		return "", err
	}
	v, err := m.conn.Object("org.freedesktop.systemd1", path).GetProperty("org.freedesktop.systemd1.Unit.ActiveState")
	if err != nil {
		return "", err
	}
	s, _ := v.Value().(string)
	return s, nil
}

// FailedUnits lists units in the failed state.
func (m *DBusManager) FailedUnits(ctx context.Context) ([]string, error) {
	var units []struct {
		Name, Description, LoadState, ActiveState, SubState, Following string
		Path                                                           dbus.ObjectPath
		JobID                                                          uint32
		JobType                                                        string
		JobPath                                                        dbus.ObjectPath
	}
	if err := m.systemd().CallWithContext(ctx, "org.freedesktop.systemd1.Manager.ListUnitsFiltered", 0, []string{"failed"}).Store(&units); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(units))
	for _, u := range units {
		out = append(out, u.Name)
	}
	return out, nil
}

// TimeSyncedFile is created by systemd-timesyncd once NTP has synchronised.
var TimeSyncedFile = "/run/systemd/timesync/synchronized"

// TimeTrusted reports whether the clock can be used for schedules: NTP has
// synchronised, or the agent set it from the backend's HTTPS Date header.
func TimeTrusted(runDir string) bool {
	for _, p := range []string{TimeSyncedFile, filepath.Join(runDir, "time-from-backend")} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// SetClockFromBackend sets the system clock from a trusted HTTPS Date header
// when NTP hasn't synchronised (UDP 123 blocked). Needs CAP_SYS_TIME.
func SetClockFromBackend(runDir string, serverTime time.Time) error {
	if _, err := os.Stat(TimeSyncedFile); err == nil {
		return nil
	}
	if serverTime.IsZero() {
		return errors.New("no server time")
	}
	diff := time.Until(serverTime)
	if diff < 0 {
		diff = -diff
	}
	if diff > 2*time.Second {
		if err := setClock(serverTime); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(runDir, "time-from-backend"), []byte(serverTime.UTC().Format(time.RFC3339)), 0o644)
}

// FactoryResetFlag is checked by castmill-data-setup at boot (wipes /data).
// It lives in the agent's state directory (the agent can't write elsewhere).
const FactoryResetFlag = "factory-reset"

// RequestFactoryReset sets the flag in stateDir (/data/castmill/state).
func RequestFactoryReset(stateDir string) error {
	p := filepath.Join(stateDir, FactoryResetFlag)
	if err := os.WriteFile(p, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644); err != nil {
		return err
	}
	if f, err := os.Open(p); err == nil {
		_ = f.Sync()
		f.Close()
	}
	return nil
}
