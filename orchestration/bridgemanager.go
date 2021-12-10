package orchestration

import (
	"fmt"
	"sync"

	"github.com/danielpaulus/go-adb/adb"
	log "github.com/sirupsen/logrus"
)

//BridgeManager listens to device attached or removed events.
//It will auto start a new Bridge for every connected device.
//Depending on the launch arguments, bridges can either be executed
//in a separate process by starting a subprocessbridge or in the same process
//by just starting a regular UsbTcpBridge.
type BridgeManager struct {
	devices          []adb.DeviceInfo
	basePort         int
	currentPort      int
	bridges          []Bridge
	processPerDevice bool
	mux              sync.Mutex
	closed           bool
	bridgeProcess    string
}

//Bridge is the basic interface for a struct that will bridge USB data to a TCP port.
//Currently there is the adb/usb_tcp_bridge.go implementation that calls libusb directly and
//the adb/subprocess_bridge.go which wraps libusb into a separate process.
type Bridge interface {
	Close() error
	GetStateName() string
	GetSerialNumber() string
	Start() error
}

//NewSubProcessBridgeManager will spawn a new process for every device using the subprocessbridge.
//The first device will be on 0.0.0.0:basePort, the second one on basePort+1 then basePort+2 etc.
func NewSubProcessBridgeManager(execName string, basePort int) *BridgeManager {
	return &BridgeManager{currentPort: basePort, basePort: basePort, bridges: make([]Bridge, 0), devices: make([]adb.DeviceInfo, 0),
		processPerDevice: true, closed: false, bridgeProcess: execName}
}

//NewBridgeManager starts one process go-adb. USB Code will be used directly in this process for all devices.
//The first device will be on 0.0.0.0:basePort, the second one on basePort+1 then basePort+2 etc.
func NewBridgeManager(basePort int) *BridgeManager {
	return &BridgeManager{currentPort: basePort, basePort: basePort, bridges: make([]Bridge, 0), devices: make([]adb.DeviceInfo, 0),
		processPerDevice: false, closed: false}
}

//DeviceAdded should be called externally by a DeviceDetector, currently it will start a new Bridge for unknown devices
//or do nothing for devices it already started a Bridge for.
func (b *BridgeManager) DeviceAdded(newDevice adb.DeviceInfo) {
	if isIn(b.devices, newDevice) {
		return
	}

	//prevent the tiny chance of a race condition that if someone attaches a new device,
	//while a Bridgemanager is closed, we might end up starting a bridge
	//during shutdown
	b.mux.Lock()
	defer b.mux.Unlock()
	if b.closed {
		return
	}

	b.devices = append(b.devices, newDevice)
	b.startBridge(newDevice)
	return
}

//DeviceRemoved is currently a no-op
func (b *BridgeManager) DeviceRemoved(removedDevice adb.DeviceInfo) {
}

//InitialList should be called once externally by a DeviceDetector, it will start a new Bridge for every device
//contained in the initial list.
func (b *BridgeManager) InitialList(currentlyConnected []adb.DeviceInfo) {
	b.devices = currentlyConnected
	for _, dev := range currentlyConnected {
		b.startBridge(dev)
	}
}

func createBridge(device adb.DeviceInfo, port int) Bridge {
	return adb.NewUsbTcpBridge(device, port)
}

func (b *BridgeManager) startBridge(device adb.DeviceInfo) {
	var bridge Bridge
	if b.processPerDevice {
		bridge = adb.NewSubProcessBridge(device, b.currentPort, b.bridgeProcess)
	} else {
		bridge = createBridge(device, b.currentPort)
	}
	log.WithFields(log.Fields{"device": device.SerialNumber, "port": b.currentPort}).Info("starting usb-bridge")
	b.currentPort++
	b.bridges = append(b.bridges, bridge)
	bridge.Start()
}

//BridgeList returns a list of map[string]interface{} that can be converted to JSON easily containing
//the USB serial, the port and the current state of each bridge.
func (b *BridgeManager) BridgeList() []map[string]interface{} {
	result := make([]map[string]interface{}, len(b.bridges))
	for i, bridge := range b.bridges {
		bridgeData := make(map[string]interface{})
		bridgeData["serial"] = bridge.GetSerialNumber()
		bridgeData["port"] = b.basePort + i
		bridgeData["state"] = bridge.GetStateName()
		result[i] = bridgeData
	}
	return result
}

//Close shuts down all bridges gracefully
//A call to Close is idempotent.
func (b *BridgeManager) Close() error {
	b.mux.Lock()
	defer b.mux.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	var closeErr error
	for _, bridge := range b.bridges {
		err := bridge.Close()
		if err != nil {
			if closeErr == nil {
				closeErr = err
			} else {
				closeErr = fmt.Errorf("%+v ; %+v", closeErr, err)
			}
		}
	}
	return closeErr
}
