package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/castmill/castmill-os/agent/internal/config"
	"github.com/castmill/castmill-os/agent/internal/provision"
	"github.com/castmill/castmill-os/agent/internal/wifi"
)

// runProvisionUSB applies castmill.conf from a USB stick (started by udev
// for each USB partition). Only while the device is offline, like the
// on-screen Wi-Fi setup; the file stays on the stick for the next device.
func runProvisionUSB(cfg *config.Agent, log *slog.Logger, dev string) error {
	if !strings.HasPrefix(dev, "/dev/") {
		return fmt.Errorf("usage: castmill-agent provision-usb /dev/<partition>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	nm, err := wifi.Connect()
	if err != nil {
		return fmt.Errorf("NetworkManager: %w", err)
	}
	defer nm.Close()
	if online, err := nm.Online(ctx); err != nil {
		return fmt.Errorf("network status: %w", err)
	} else if online {
		log.Info("USB stick ignored: castmill.conf is only applied from USB while the device is offline", "device", dev)
		return nil
	}

	dir := filepath.Join("/run/castmill-usb", filepath.Base(dev))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	defer os.Remove(dir)
	flags := uintptr(unix.MS_RDONLY | unix.MS_NOSUID | unix.MS_NODEV | unix.MS_NOEXEC)
	var mountErr error
	for _, fstype := range []string{"vfat", "exfat"} {
		if mountErr = unix.Mount(dev, dir, fstype, flags, ""); mountErr == nil {
			break
		}
	}
	if mountErr != nil {
		log.Info("USB partition not readable (not FAT/exFAT?)", "device", dev, "err", mountErr)
		return nil
	}
	defer unix.Unmount(dir, 0)

	conf := filepath.Join(dir, "castmill.conf")
	if _, err := os.Stat(conf); err != nil {
		return nil // an ordinary USB stick
	}
	p := provisionPaths(cfg, []string{conf})
	p.Keep = true
	res, err := provision.Apply(p, cfg.Backend.AllowOverride, log)
	if err != nil {
		return fmt.Errorf("castmill.conf on %s: %w", dev, err)
	}
	log.Info("castmill.conf applied from USB stick", "device", dev, "applied", strings.Join(res.Applied, ","))
	if slices.Contains(res.Applied, "wifi") {
		if err := nm.ReloadConnections(ctx); err != nil {
			log.Warn("NetworkManager reload", "err", err)
		}
	}
	if slices.Contains(res.Applied, "proxy") || slices.Contains(res.Applied, "ntp") {
		log.Info("proxy and NTP settings take effect after a reboot")
	}
	return nil
}
