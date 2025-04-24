//go:build !linux

package container

import (
	"fmt"
	"runtime"

	devicecfg "github.com/L-F-Z/cri-t/internal/config/device"
)

func (c *container) SpecAddDevices(configuredDevices, annotationDevices []devicecfg.Device, privilegedWithoutHostDevices, enableDeviceOwnershipFromSecurityContext bool) error {
	return fmt.Errorf("(*container).SpecAddDevices not supported on %s", runtime.GOOS)
}
