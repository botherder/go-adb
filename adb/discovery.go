package adb

import (
	"fmt"

	"github.com/google/gousb"

	log "github.com/sirupsen/logrus"
)

const adbInterfaceProtocol gousb.Protocol = 0x1
const adbInterfaceSubclass gousb.Class = 0x42

//DeviceInfo contains all relevant information we can get from USB for one Android device.
type DeviceInfo struct {
	SerialNumber string
	ProductName  string
	VID          gousb.ID
	PID          gousb.ID
	UsbInfo      string
}

//ListDevices looks for physical Android devices connected to the USB host and returns a slice of AndroidDeviceInfo or an error.
func ListDevices() ([]DeviceInfo, error) {
	ctx := gousb.NewContext()
	defer func() {
		err := ctx.Close()
		if err != nil {
			log.Warnf("listDevices failed closing context with err:%+v", err)
		}
	}()
	return findDevices(ctx)
}

func findDevices(ctx *gousb.Context) ([]DeviceInfo, error) {
	devices, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		// this function is called for every device present.
		// Returning true means the device should be opened.
		return isAndroidDevice(desc)
	})
	defer closeDevices(devices)

	mappedDevices, mapErr := mapToAndroidDevice(devices)

	if err != nil || mapErr != nil {
		//according to gousb docs, an error could mean only one device failed to open but the others are fine
		//in that case we can continue as normal but should log a warning
		//if we return the error here, one broken android device would prevent us from seeing the non broken ones.
		return mappedDevices, fmt.Errorf("error getting devicelist. it could be that devices are missing from go-adb due to broken devices or file system permission issues. check that the adb_user has access to all devices. error getting list:%v error mapping devices: %v", err, mapErr)
	}

	return mappedDevices, nil
}

func closeDevices(devices []*gousb.Device) {
	for _, dev := range devices {
		err := dev.Close()
		if err != nil {
			log.Warnf("Error %v while closing Android device:%+v", err, dev)
		}
	}
}

func isAndroidDevice(deviceDesc *gousb.DeviceDesc) bool {
	for _, configDesc := range deviceDesc.Configs {
		for _, iface := range configDesc.Interfaces {
			if isAdbInterface(iface) {
				return true
			}
		}
	}
	return false
}

func isAdbInterface(iface gousb.InterfaceDesc) bool {
	for _, alt := range iface.AltSettings {
		if alt.Class == gousb.ClassVendorSpec &&
			alt.SubClass == adbInterfaceSubclass &&
			alt.Protocol == adbInterfaceProtocol {
			return verifyAdbEndpointsPresent(alt)
		}
	}
	return false
}

func verifyAdbEndpointsPresent(alt gousb.InterfaceSetting) bool {
	if len(alt.Endpoints) != 2 {
		return false
	}
	in, out := false, false
	for _, v := range alt.Endpoints {
		in = in || (v.Direction == gousb.EndpointDirectionIn)
		out = out || v.Direction == gousb.EndpointDirectionOut
	}
	return in && out
}

func mapToAndroidDevice(devices []*gousb.Device) ([]DeviceInfo, error) {
	androidDevices := make([]DeviceInfo, 0)
	var lastErr error = nil
	for _, device := range devices {
		log.Tracef("Getting serial for: %s", device.String())
		serial, err := device.SerialNumber()
		if err != nil {
			log.Warnf("error getting serial: %v skipping device", err)
			lastErr = err
			continue
		}
		log.Tracef("Got serial: %s", serial)

		log.Tracef("Getting product name for: %s", device.String())
		product, err := device.Product()
		if err != nil {
			log.Warnf("error getting serial: %v skipping device", err)
			lastErr = err
			continue
		}
		log.Tracef("Got product name: %s", product)

		androidDevices = append(androidDevices, DeviceInfo{
			SerialNumber: serial,
			ProductName:  product,
			VID:          device.Desc.Vendor,
			PID:          device.Desc.Product,
			UsbInfo:      device.String()})
	}
	return androidDevices, lastErr
}
