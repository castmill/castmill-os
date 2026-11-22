//go:build !linux

package system

import (
	"errors"
	"time"
)

func setClock(time.Time) error { return errors.New("setting the clock is only supported on Linux") }
