package system

import (
	"time"

	"golang.org/x/sys/unix"
)

func setClock(t time.Time) error {
	tv := unix.NsecToTimeval(t.UnixNano())
	return unix.Settimeofday(&tv)
}
