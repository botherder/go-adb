package adb

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/pkg/errors"

	"github.com/google/gousb"
	log "github.com/sirupsen/logrus"
)

//UsbAdapter reads and writes from AV Quicktime USB Bulk endpoints
type UsbAdapter struct {
	outEndpoint   *gousb.OutEndpoint
	inEndpoint    *gousb.InEndpoint
	adbInterface  *gousb.Interface
	usbDevice     *gousb.Device
	usbContext    *gousb.Context
	usbConfig     *gousb.Config
	stopSignal    chan interface{}
	Dump          bool
	DumpOutWriter io.Writer
	DumpInWriter  io.Writer

	packetChannel chan Packet
	errorChannel  chan error

	writeChannel      chan Packet
	writeErrorChannel chan error
	writeDone         chan interface{}
	injectedLog       *log.Entry
}

func (usbAdapter *UsbAdapter) log() *log.Entry {
	return usbAdapter.injectedLog
}

func (usbAdapter *UsbAdapter) Read(p []byte) (int, error) {
	n, err := usbAdapter.inEndpoint.Read(p)
	return n, err
}

//WriteDataToUsb implements the UsbWriter interface and sends the byte array to the usb bulk endpoint.
func (usbAdapter *UsbAdapter) Write(bytes []byte) (int, error) {
	toContext, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
	defer cancel()
	n, err := usbAdapter.outEndpoint.WriteContext(toContext, bytes)
	if usbAdapter.Dump {
		_, err := usbAdapter.DumpOutWriter.Write(bytes)
		if err != nil {
			usbAdapter.log().Fatalf("Failed dumping data:%v", err)
		}
	}
	return n, err
}

func (usbAdapter *UsbAdapter) Close() {

	usbAdapter.log().Info("Closing usbadapter")

	if usbAdapter.adbInterface != nil {
		log.Info("stopping write loop..")
		go func() { usbAdapter.stopSignal <- struct{}{} }()

		select {
		case <-usbAdapter.writeDone:
			log.Info("write loop stopped")
		case <-time.After(time.Second * 5):
			log.Warn("timed out waiting for write loop to finish")
		}

		usbAdapter.log().Info("Closing adb interface")
		usbAdapter.adbInterface.Close()
	}

	if usbAdapter.usbConfig != nil {
		usbAdapter.log().Debug("Closing usb config")
		err := usbAdapter.usbConfig.Close()
		if err != nil {
			usbAdapter.log().Warnf("Error closing context %+v", err)
		}
	}

	if usbAdapter.usbDevice != nil {
		usbAdapter.log().Debug("Closing usbdevice")
		err := usbAdapter.usbDevice.Close()
		if err != nil {
			usbAdapter.log().Warnf("Error closing usbdevice %+v", err)
		}
	}

	usbAdapter.log().Debug("Closing usb context")
	if usbAdapter.usbContext != nil {
		err := usbAdapter.usbContext.Close()
		//context will deadlock if closed twice
		usbAdapter.usbContext = nil
		if err != nil {
			usbAdapter.log().Warnf("Error closing context %+v", err)
		}
	}
	usbAdapter.log().Info("usbadapter closed")

}

func (usbAdapter *UsbAdapter) ConnectDevice(device DeviceInfo) error {
	ctx := gousb.NewContext()
	usbAdapter.usbContext = ctx
	usbDevice, err := OpenDevice(ctx, device)
	if err != nil {
		return err
	}
	usbDevice.SetAutoDetach(true)
	usbAdapter.usbDevice = usbDevice
	usbAdapter.log().Debug("device open")
	confignum, _ := usbDevice.ActiveConfigNum()
	usbAdapter.log().Debugf("Config is active: %d", confignum)

	config, err := usbDevice.Config(confignum)
	if err != nil {
		return errors.New("Could not retrieve config")
	}
	usbAdapter.usbConfig = config

	usbAdapter.log().Debugf("Config is active: %s", config.String())

	iface, err := findAndClaimAdbInterface(config)
	if err != nil {
		usbAdapter.log().Debug("could not get adb Interface")
		return err
	}
	usbAdapter.log().Debugf("Got adb iface:%s", iface.String())

	inboundBulkEndpointIndex, _, err := findBulkEndpoint(iface.Setting, gousb.EndpointDirectionIn)
	if err != nil {
		return err
	}

	outboundBulkEndpointIndex, _, err := findBulkEndpoint(iface.Setting, gousb.EndpointDirectionOut)
	if err != nil {
		return err
	}

	inEndpoint, err := iface.InEndpoint(inboundBulkEndpointIndex)
	if err != nil {
		usbAdapter.log().Error("couldnt get InEndpoint")
		return err
	}
	usbAdapter.log().Debugf("Inbound Bulk: %s", inEndpoint.String())

	outEndpoint, err := iface.OutEndpoint(outboundBulkEndpointIndex)
	if err != nil {
		usbAdapter.log().Error("couldnt get OutEndpoint")
		return err
	}
	usbAdapter.log().Debugf("Outbound Bulk: %s", outEndpoint.String())
	usbAdapter.outEndpoint = outEndpoint

	if err != nil {
		usbAdapter.log().Error("couldnt create stream")
		return err
	}
	usbAdapter.log().Debug("Endpoint claimed")
	usbAdapter.log().Infof("Device '%s' USB connection ready", device.SerialNumber)
	usbAdapter.inEndpoint = inEndpoint

	usbAdapter.adbInterface = iface
	return nil
}

func findBulkEndpoint(setting gousb.InterfaceSetting, direction gousb.EndpointDirection) (int, gousb.EndpointAddress, error) {
	for _, v := range setting.Endpoints {
		if v.Direction == direction {
			return v.Number, v.Address, nil

		}
	}
	return 0, 0, errors.New("Inbound Bulkendpoint not found")
}

func findAndClaimAdbInterface(config *gousb.Config) (*gousb.Interface, error) {
	log.Debug("Looking for adb interface..")
	found, ifaceIndex := findInterfaceForSubclass(config.Desc, adbInterfaceSubclass)
	if !found {
		return nil, fmt.Errorf("did not find interface %v", config)
	}
	log.Debugf("Found adb: %d", ifaceIndex)
	return config.Interface(ifaceIndex, 0)
}

func findInterfaceForSubclass(confDesc gousb.ConfigDesc, subClass gousb.Class) (bool, int) {
	for _, iface := range confDesc.Interfaces {
		for _, alt := range iface.AltSettings {
			isVendorClass := alt.Class == gousb.ClassVendorSpec
			isCorrectSubClass := alt.SubClass == subClass
			log.Debugf("iface:%v altsettings:%d isvendor:%t isub:%t", iface, len(iface.AltSettings), isVendorClass, isCorrectSubClass)
			if isVendorClass && isCorrectSubClass {
				return true, iface.Number
			}
		}
	}
	return false, -1
}

//OpenDevice finds a gousb.Device by using the provided iosDevice.SerialNumber. It returns an open device handle.
//Opening using VID and PID is not specific enough, as different iOS devices can have identical VID/PID combinations.
func OpenDevice(ctx *gousb.Context, androidDevice DeviceInfo) (*gousb.Device, error) {
	deviceList, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return desc.Product == androidDevice.PID && desc.Vendor == androidDevice.VID
	})

	if err != nil {
		log.Warn("Error opening usb devices", err)
	}
	var usbDevice *gousb.Device = nil
	for _, device := range deviceList {
		sn, err := device.SerialNumber()
		if err != nil {
			log.Warn("Error retrieving Serialnumber", err)
		}
		if sn == androidDevice.SerialNumber {
			usbDevice = device
		} else {
			device.Close()
		}
	}

	if usbDevice == nil {
		return nil, fmt.Errorf("Unable to find device:%+v", androidDevice)
	}
	return usbDevice, nil
}
