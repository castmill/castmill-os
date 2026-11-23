//go:build !linux

package display

import "errors"

var errNotLinux = errors.New("display power control needs Linux")

func cecConnector(string) (uint32, uint32, error) { return 0, 0, errNotLinux }
func cecPower(string, bool) error                 { return errNotLinux }
func ddcPower(string, bool) error                 { return errNotLinux }
