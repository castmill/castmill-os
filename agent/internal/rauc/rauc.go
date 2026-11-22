// Package rauc talks to the RAUC service over D-Bus (de.pengutronix.rauc).
package rauc

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/godbus/dbus/v5"

	"github.com/castmill/castmill-os/agent/internal/api"
)

const (
	busName   = "de.pengutronix.rauc"
	objPath   = dbus.ObjectPath("/")
	installer = "de.pengutronix.rauc.Installer"
)

// Slot is one entry of GetSlotStatus.
type Slot struct {
	Name          string // e.g. rootfs.0
	Class         string // rootfs
	Bootname      string // A / B
	State         string // booted / active / inactive
	BootStatus    string // good / bad
	BundleVersion string
	Device        string
}

// Status is the slot overview.
type Status struct {
	Compatible string
	BootSlot   string // bootname of the booted slot
	Slots      []Slot
}

// Booted returns the booted rootfs slot.
func (s *Status) Booted() *Slot {
	for i := range s.Slots {
		if s.Slots[i].State == "booted" {
			return &s.Slots[i]
		}
	}
	return nil
}

// Other returns the rootfs slot that isn't booted.
func (s *Status) Other() *Slot {
	for i := range s.Slots {
		if s.Slots[i].Class == "rootfs" && s.Slots[i].State != "booted" {
			return &s.Slots[i]
		}
	}
	return nil
}

// API converts to the wire type.
func (s *Status) API() []api.Slot {
	out := make([]api.Slot, 0, len(s.Slots))
	for _, sl := range s.Slots {
		out = append(out, api.Slot{Name: sl.Name, Bootname: sl.Bootname, State: sl.State,
			BootStatus: sl.BootStatus, BundleVersion: sl.BundleVersion})
	}
	return out
}

// Progress is reported while installing.
type Progress struct {
	Percent int
	Message string
}

// InstallOptions are passed to InstallBundle.
type InstallOptions struct {
	TLSCert string // client certificate for streaming over mTLS
	TLSKey  string
	TLSCA   string
}

// Client is the subset of RAUC the agent and health check use.
type Client interface {
	Status(ctx context.Context) (*Status, error)
	// Install blocks until the installation finishes. progress may be nil.
	Install(ctx context.Context, source string, opts InstallOptions, progress func(Progress)) error
	// Mark sets "good" or "bad" on "booted" / "other" / a slot name.
	Mark(ctx context.Context, state, slot string) error
}

// DBus is the real client.
type DBus struct{ conn *dbus.Conn }

// Connect opens the system bus.
func Connect() (*DBus, error) {
	c, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	return &DBus{conn: c}, nil
}

// Close releases the bus connection.
func (d *DBus) Close() error { return d.conn.Close() }

func (d *DBus) obj() dbus.BusObject { return d.conn.Object(busName, objPath) }

// Status implements Client.
func (d *DBus) Status(ctx context.Context) (*Status, error) {
	o := d.obj()
	st := &Status{}
	if v, err := o.GetProperty(installer + ".Compatible"); err == nil {
		st.Compatible, _ = v.Value().(string)
	}
	if v, err := o.GetProperty(installer + ".BootSlot"); err == nil {
		st.BootSlot, _ = v.Value().(string)
	}
	var raw []struct {
		Name  string
		Props map[string]dbus.Variant
	}
	if err := o.CallWithContext(ctx, installer+".GetSlotStatus", 0).Store(&raw); err != nil {
		return nil, fmt.Errorf("rauc GetSlotStatus: %w", err)
	}
	for _, r := range raw {
		str := func(k string) string {
			if v, ok := r.Props[k]; ok {
				s, _ := v.Value().(string)
				return s
			}
			return ""
		}
		st.Slots = append(st.Slots, Slot{
			Name: r.Name, Class: str("class"), Bootname: str("bootname"), State: str("state"),
			BootStatus: str("boot-status"), BundleVersion: str("bundle.version"), Device: str("device"),
		})
	}
	sort.Slice(st.Slots, func(i, j int) bool { return st.Slots[i].Name < st.Slots[j].Name })
	return st, nil
}

// Install implements Client.
func (d *DBus) Install(ctx context.Context, source string, opts InstallOptions, progress func(Progress)) error {
	sigs := make(chan *dbus.Signal, 16)
	d.conn.Signal(sigs)
	defer d.conn.RemoveSignal(sigs)
	match := []dbus.MatchOption{dbus.WithMatchObjectPath(objPath), dbus.WithMatchSender(busName)}
	if err := d.conn.AddMatchSignal(append(match, dbus.WithMatchInterface(installer), dbus.WithMatchMember("Completed"))...); err != nil {
		return err
	}
	defer d.conn.RemoveMatchSignal(append(match, dbus.WithMatchInterface(installer), dbus.WithMatchMember("Completed"))...)
	propsMatch := append(match, dbus.WithMatchInterface("org.freedesktop.DBus.Properties"), dbus.WithMatchMember("PropertiesChanged"))
	if err := d.conn.AddMatchSignal(propsMatch...); err != nil {
		return err
	}
	defer d.conn.RemoveMatchSignal(propsMatch...)

	args := map[string]dbus.Variant{}
	if opts.TLSCert != "" {
		args["tls-cert"] = dbus.MakeVariant(opts.TLSCert)
		args["tls-key"] = dbus.MakeVariant(opts.TLSKey)
	}
	if opts.TLSCA != "" {
		args["tls-ca"] = dbus.MakeVariant(opts.TLSCA)
	}
	if err := d.obj().CallWithContext(ctx, installer+".InstallBundle", 0, source, args).Err; err != nil {
		return fmt.Errorf("rauc InstallBundle: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case s := <-sigs:
			switch s.Name {
			case installer + ".Completed":
				code, _ := s.Body[0].(int32)
				if code == 0 {
					return nil
				}
				msg := "installation failed"
				if v, err := d.obj().GetProperty(installer + ".LastError"); err == nil {
					if e, _ := v.Value().(string); e != "" {
						msg = e
					}
				}
				return errors.New(msg)
			case "org.freedesktop.DBus.Properties.PropertiesChanged":
				if progress == nil || len(s.Body) < 2 {
					continue
				}
				changed, _ := s.Body[1].(map[string]dbus.Variant)
				if v, ok := changed["Progress"]; ok {
					var p struct {
						Percent int32
						Message string
						Depth   int32
					}
					if err := dbus.Store([]any{v.Value()}, &p); err == nil {
						progress(Progress{Percent: int(p.Percent), Message: p.Message})
					}
				}
			}
		}
	}
}

// Mark implements Client.
func (d *DBus) Mark(ctx context.Context, state, slot string) error {
	var name, msg string
	if err := d.obj().CallWithContext(ctx, installer+".Mark", 0, state, slot).Store(&name, &msg); err != nil {
		return fmt.Errorf("rauc mark-%s %s: %w", state, slot, err)
	}
	return nil
}
