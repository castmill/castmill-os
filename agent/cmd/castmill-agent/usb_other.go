//go:build !linux

package main

import (
	"errors"
	"log/slog"

	"github.com/castmill/castmill-os/agent/internal/config"
)

func runProvisionUSB(*config.Agent, *slog.Logger, string) error {
	return errors.New("provision-usb is only available on Linux")
}
