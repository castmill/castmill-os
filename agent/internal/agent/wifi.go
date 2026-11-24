package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/castmill/castmill-os/agent/internal/wifi"
)

// On-screen Wi-Fi setup: the kiosk shell's status page (a device that never
// loaded the device app) and the Wi-Fi entry of the device app's menu (a
// device playing from its cache), both through the kiosk shell. Changes only
// while the device is offline: a screen that works can't have its network
// changed by whoever plugs in a keyboard or touches it.

// WiFiStatus answers wifi_status.
type WiFiStatus struct {
	Available bool           `json:"available"` // the device has Wi-Fi
	Online    bool           `json:"online"`
	Current   *wifi.Network  `json:"current,omitempty"` // the network it's connected to
	Networks  []wifi.Network `json:"networks"`          // in range; only while offline
	Error     string         `json:"error,omitempty"`
}

var errOnline = errors.New("the device is online; Wi-Fi can only be set up here while it's offline")

func (a *Agent) wifiStatus(ctx context.Context) (WiFiStatus, error) {
	if a.WiFi == nil {
		return WiFiStatus{Networks: []wifi.Network{}}, nil
	}
	online, err := a.WiFi.Online(ctx)
	if err != nil {
		return WiFiStatus{}, fmt.Errorf("network status: %w", err)
	}
	s := WiFiStatus{Available: true, Online: online, Networks: []wifi.Network{}}
	current, err := a.WiFi.Current(ctx)
	if errors.Is(err, wifi.ErrNoWiFi) {
		s.Available = false // e.g. online over Ethernet, without a Wi-Fi adapter
		return s, nil
	}
	s.Current = current
	if online {
		return s, nil
	}
	nets, err := a.WiFi.Scan(ctx)
	if err != nil {
		s.Available = false
		s.Error = err.Error()
		return s, nil
	}
	s.Networks = nets
	return s, nil
}

func (a *Agent) wifiConnect(ctx context.Context, output string, args json.RawMessage) error {
	if a.WiFi == nil {
		return errors.New("this device has no Wi-Fi")
	}
	var req struct {
		SSID   string `json:"ssid"`
		PSK    string `json:"psk"`
		Hidden bool   `json:"hidden"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return fmt.Errorf("wifi_connect: %w", err)
	}
	if err := wifi.Validate(req.SSID, req.PSK); err != nil {
		return err
	}
	online, err := a.WiFi.Online(ctx)
	if err != nil {
		return fmt.Errorf("network status: %w", err)
	}
	if online {
		return errOnline
	}
	a.Log.Info("joining Wi-Fi from the on-screen setup", "output", output, "ssid", req.SSID)
	if err := a.WiFi.Join(ctx, req.SSID, req.PSK, req.Hidden); err != nil {
		a.Log.Warn("on-screen Wi-Fi setup failed", "ssid", req.SSID, "err", err)
		return err
	}
	return nil
}
