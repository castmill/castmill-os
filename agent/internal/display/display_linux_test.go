package display

import "testing"

// Values from linux/cec.h as the kernel builds them (and strace prints them).
func TestIoctlNumbers(t *testing.T) {
	for name, c := range map[string]struct{ got, want uintptr }{
		"CEC_ADAP_G_PHYS_ADDR":      {cecAdapGPhysAddr, 0x80026101},
		"CEC_ADAP_G_LOG_ADDRS":      {cecAdapGLogAddrs, 0x805c6103},
		"CEC_ADAP_S_LOG_ADDRS":      {cecAdapSLogAddrs, 0xc05c6104},
		"CEC_TRANSMIT":              {cecTransmit, 0xc0386105},
		"CEC_ADAP_G_CONNECTOR_INFO": {cecAdapGConnectorInfo, 0x8044610a},
	} {
		if c.got != c.want {
			t.Errorf("%s = %#x, want %#x", name, c.got, c.want)
		}
	}
}
