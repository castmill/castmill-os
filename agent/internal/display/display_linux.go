package display

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ioctl request numbers (asm-generic _IOC encoding, same on arm64 and x86-64).
func ioc(dir, typ, nr, size uintptr) uintptr { return dir<<30 | size<<16 | typ<<8 | nr }

const (
	iocWrite = 1
	iocRead  = 2
)

var (
	cecAdapGPhysAddr      = ioc(iocRead, 'a', 1, 2)
	cecAdapGLogAddrs      = ioc(iocRead, 'a', 3, sizeofCECLogAddrs)
	cecAdapSLogAddrs      = ioc(iocRead|iocWrite, 'a', 4, sizeofCECLogAddrs)
	cecTransmit           = ioc(iocRead|iocWrite, 'a', 5, sizeofCECMsg)
	cecAdapGConnectorInfo = ioc(iocRead, 'a', 10, sizeofCECConnInfo)
)

const i2cSlave = 0x0703

func ioctl(f *os.File, req uintptr, buf []byte) error {
	_, _, e := unix.Syscall(unix.SYS_IOCTL, f.Fd(), req, uintptr(unsafe.Pointer(&buf[0])))
	if e != 0 {
		return e
	}
	return nil
}

// cecConnector returns the DRM card and connector ID a CEC adapter belongs to.
func cecConnector(dev string) (card, connector uint32, err error) {
	f, err := os.OpenFile(dev, os.O_RDWR|unix.O_NONBLOCK, 0)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	b := make([]byte, sizeofCECConnInfo)
	if err := ioctl(f, cecAdapGConnectorInfo, b); err != nil {
		return 0, 0, err
	}
	if binary.NativeEndian.Uint32(b[0:]) != 1 { // CEC_CONNECTOR_TYPE_DRM
		return 0, 0, errors.New("not a DRM connector")
	}
	return binary.NativeEndian.Uint32(b[4:]), binary.NativeEndian.Uint32(b[8:]), nil
}

// cecPower sends Standby, or Image View On and Active Source, to the TV.
func cecPower(dev string, on bool) error {
	f, err := os.OpenFile(dev, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	la := byte(cecUnregistered)
	las := make([]byte, sizeofCECLogAddrs)
	if err := ioctl(f, cecAdapGLogAddrs, las); err != nil {
		return fmt.Errorf("logical addresses: %w", err)
	}
	if binary.NativeEndian.Uint16(las[4:]) == 0 && las[7] == 0 {
		// Nobody configured the adapter: claim a Playback address. Blocks
		// while claiming; returns at once while the hotplug line is low.
		cfg := cecLogAddrs()
		if err := ioctl(f, cecAdapSLogAddrs, cfg); err != nil && !errors.Is(err, unix.EBUSY) {
			return fmt.Errorf("claim a logical address: %w", err)
		}
		_ = ioctl(f, cecAdapGLogAddrs, las)
	}
	if binary.NativeEndian.Uint16(las[4:]) != 0 { // log_addr_mask
		la = las[0]
	}
	pa := make([]byte, 2)
	physAddr := uint16(cecPhysAddrNone)
	if ioctl(f, cecAdapGPhysAddr, pa) == nil {
		physAddr = binary.NativeEndian.Uint16(pa)
	}
	if !on && la == cecUnregistered {
		return errors.New("no logical address (is the display connected?)")
	}

	for i, m := range cecMessages(on, la, physAddr) {
		b := cecMsg(m...)
		if err := ioctl(f, cecTransmit, b); err != nil {
			return fmt.Errorf("transmit %#x: %w", m[1], err)
		}
		if cecTxStatus(b)&cecTxStatusOK == 0 {
			if i > 0 {
				break // the TV is on; switching its input is best effort
			}
			return fmt.Errorf("the display didn't acknowledge %#x (tx status %#x)", m[1], cecTxStatus(b))
		}
	}
	return nil
}

// ddcPower sets the monitor's power mode (VCP 0xD6) over DDC/CI.
func ddcPower(dev string, on bool) error {
	f, err := os.OpenFile(dev, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, _, e := unix.Syscall(unix.SYS_IOCTL, f.Fd(), i2cSlave, ddcAddr); e != 0 {
		return fmt.Errorf("select DDC/CI address: %w", e)
	}
	value := uint16(vcpPowerOffDP)
	if on {
		value = vcpPowerOn
	}
	if _, err := f.Write(ddcSetVCP(vcpPowerMode, value)); err != nil {
		return fmt.Errorf("the monitor didn't accept DDC/CI: %w", err)
	}
	time.Sleep(50 * time.Millisecond) // MCCS: wait before the next command
	return nil
}
