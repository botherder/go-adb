package orchestration

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/danielpaulus/go-adb/adb"
	log "github.com/sirupsen/logrus"
)

//DeviceUpdateListener defines the interface you need to implement to get notified about
//devices being added or removed from the host.
type DeviceUpdateListener interface {
	//A new device was plugged int
	DeviceAdded(newDevice adb.DeviceInfo)
	//A device was removed
	DeviceRemoved(removedDevice adb.DeviceInfo)
	//Sent once when you register, so you have the current list
	InitialList(currentlyConnected []adb.DeviceInfo)
}

//DeviceDetector keeps track of devices being added and removed from the host
//by periodically scanning the devicelist and notifying all listeners about changes.
type DeviceDetector struct {
	listeners    []DeviceUpdateListener
	devices      []adb.DeviceInfo
	mux          sync.Mutex
	logCounter   int
	done         chan struct{}
	deviceLister func() ([]adb.DeviceInfo, error)
}

//NewDeviceDetector creates a new detector that checks for new devices every 5s using
//libusb directly.
func NewDeviceDetector() *DeviceDetector {
	return &DeviceDetector{listeners: make([]DeviceUpdateListener, 0),
		devices:      make([]adb.DeviceInfo, 0),
		done:         make(chan struct{}, 0),
		deviceLister: adb.ListDevices}
}

//NewProcessDeviceDetector checks for new devices every 5s by calling go-adb listdevices.
func NewProcessDeviceDetector(execpath string) *DeviceDetector {
	return &DeviceDetector{listeners: make([]DeviceUpdateListener, 0),
		devices:      make([]adb.DeviceInfo, 0),
		done:         make(chan struct{}, 0),
		deviceLister: processList(execpath)}
}

type deviceResponse struct {
	Devicelist []adb.DeviceInfo
	Err        string
}

func processList(execName string) func() ([]adb.DeviceInfo, error) {
	return func() ([]adb.DeviceInfo, error) {
		cmd := exec.Command(execName,
			"listdevices")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return []adb.DeviceInfo{}, fmt.Errorf("error:%v; output: %s", err, output)
		}

		var response deviceResponse
		lines := strings.Split(string(output), "\n")
		deviceJson := lines[len(lines)-1]
		err = json.Unmarshal([]byte(deviceJson), &response)
		if err != nil {
			return []adb.DeviceInfo{}, fmt.Errorf("processList failed decoding json with error:%v; listdevices output:%s", err, output)
		}
		err = nil
		if response.Err != "<nil>" {
			err = errors.New(response.Err)
		}
		return response.Devicelist, err
	}
}

//AddListener register your listener, you will receive the current list of devices once
//and then be updated when new devices are plugged in or devices are removed.
func (d *DeviceDetector) AddListener(listener DeviceUpdateListener) []adb.DeviceInfo {
	d.mux.Lock()
	defer d.mux.Unlock()
	d.listeners = append(d.listeners, listener)
	listener.InitialList(d.devices)
	return d.devices
}

//StartListening starts the deviceDetector so it will grab a list of devices every 5 seconds.
//It will update all listeners when devices are added or removed.
func (d *DeviceDetector) StartListening() {
	go func() {
		for {
			select {
			case <-d.done:
				break
			case <-time.After(5 * time.Second):
				d.detect()
			}

		}
	}()
}

func (d *DeviceDetector) detect() {
	d.mux.Lock()
	defer d.mux.Unlock()
	devices, err := d.deviceLister()
	if err != nil {
		log.Warnf("Error getting devicelist: %+v", err)
	}
	d.logCounter++
	if d.logCounter > 2 {
		log.Infof("detected devices: %+v", devices)
		d.logCounter = 0
	}

	for _, newDevice := range devices {
		if !isIn(d.devices, newDevice) {
			d.devices = append(d.devices, newDevice)
			notifyAddListeners(d, newDevice)
		}
	}

	for _, device := range d.devices {
		if !isIn(devices, device) {
			d.devices = remove(d.devices, device)
			notifyRemoveListeners(d, device)
		}
	}
}

func remove(devices []adb.DeviceInfo, otherDevice adb.DeviceInfo) []adb.DeviceInfo {
	index := findIn(devices, otherDevice)
	if index == -1 {
		return devices
	}
	return append(devices[:index], devices[index+1:]...)
}

func notifyAddListeners(d *DeviceDetector, newDevice adb.DeviceInfo) {
	for _, l := range d.listeners {
		l.DeviceAdded(newDevice)
	}
}
func notifyRemoveListeners(d *DeviceDetector, removedDevice adb.DeviceInfo) {
	for _, l := range d.listeners {
		l.DeviceRemoved(removedDevice)
	}
}

func isIn(devices []adb.DeviceInfo, otherDevice adb.DeviceInfo) bool {
	return findIn(devices, otherDevice) != -1
}

func findIn(devices []adb.DeviceInfo, otherDevice adb.DeviceInfo) int {
	for index, device := range devices {
		if device.SerialNumber == otherDevice.SerialNumber {
			return index
		}
	}
	return -1
}

//Close stops the device discovery loop. Please only call once.
func (d *DeviceDetector) Close() {
	d.done <- struct{}{}
}
