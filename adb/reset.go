package adb

import (
	"fmt"

	"github.com/google/gousb"
	log "github.com/sirupsen/logrus"
)

func ResetBySerial(serial string) error {
	ctx := gousb.NewContext()
	defer ctx.Close()
	return resetDevice(ctx, serial)
}

func ResetByVIDPID(vid int, pid int) error {
	ctx := gousb.NewContext()
	defer ctx.Close()
	return resetDeviceVIDPID(ctx, gousb.ID(vid), gousb.ID(pid))
}

func resetDeviceVIDPID(ctx *gousb.Context, vid gousb.ID, pid gousb.ID) error {
	devices, _ := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		// this function is called for every device present.
		// Returning true means the device should be opened.
		return desc.Vendor == vid && desc.Product == pid
	})
	defer closeDevices(devices)
	var lastErr error = nil
	for _, dev := range devices {
		lastErr = dev.Reset()
	}

	return lastErr
}

func resetDevice(ctx *gousb.Context, serial string) error {
	devices, _ := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		// this function is called for every device present.
		// Returning true means the device should be opened.
		return isAndroidDevice(desc)
	})
	defer closeDevices(devices)
	for _, dev := range devices {
		s, err := dev.SerialNumber()
		log.Warnf("could not get serial for device err:%v", err)
		if serial == s {
			return dev.Reset()
		}
	}

	return fmt.Errorf("device '%s' not found", serial)
}
