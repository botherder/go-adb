package adb

import (
	"fmt"
	"net"
	"reflect"
	"runtime"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	notInitialized = iota
	detached       = iota
	attached       = iota
	connecting     = iota
	connectedUSB   = iota
	online         = iota
	errorTCP       = iota
	errorUSB       = iota
	disconnected   = iota
)

type event struct {
	eventType int
	eventData interface{}
}

//UsbTcpBridge is a state machine working directly with a UsbAdapter for one specific device only that it will expose on one TCP port.
//It takes care of connecting to a device, accepting connections on a TCP socket
//and forwarding data between TCP and USB. It is using opQueue for transitioning between states.
type UsbTcpBridge struct {
	device       DeviceInfo
	adapter      *UsbAdapter
	tcpServer    net.Listener
	port         int
	currentState int
	opQueue      chan func()
	done         chan struct{}
	finished     chan struct{}
}

//NewUsbTcpBridge creates a new notInilialized UsbTcpBridge.
func NewUsbTcpBridge(device DeviceInfo, port int) *UsbTcpBridge {
	bridge := &UsbTcpBridge{device: device,
		port:         port,
		currentState: notInitialized,
		adapter:      &UsbAdapter{Dump: false, injectedLog: &log.Entry{}},
		opQueue:      make(chan func()),
		done:         make(chan struct{}),
		finished:     make(chan struct{}),
	}
	return bridge
}

func (u *UsbTcpBridge) log() *log.Entry {
	return log.WithFields(log.Fields{"port": u.port, "serial": u.device.SerialNumber, "state": u.GetStateName()})
}

//GetSerialNumber returns the serial usb number of the device this bridge is responsible for.
func (u *UsbTcpBridge) GetSerialNumber() string {
	return u.device.SerialNumber
}

func deviceDetached(u *UsbTcpBridge) func() {
	return func() {
		if u.currentState == errorTCP {
			u.log().WithFields(log.Fields{"current": u.GetStateName()}).Debug("skipping deviceDetachedOp")
			return
		}
		u.log().Debug("deviceDetached queuing connectUsbOp")
		u.currentState = detached
		time.Sleep(time.Second * 5)
		go func() { u.opQueue <- connectUSBOp(u) }()
	}
}

func disconnectEverything(u *UsbTcpBridge) func() {
	return func() {
		if u.currentState != online {
			u.log().WithFields(log.Fields{"current": u.GetStateName()}).Debug("skipping disconnect")
			return
		}
		u.log().Debug("Disconnecting everything")
		if u.tcpServer != nil {
			err := u.tcpServer.Close()
			if err != nil {
				u.log().Warnf("error closing tcp server %+v", err)
			}
		}

		u.adapter.Close()
		u.log().Debug("done disonnecting everything")
		u.currentState = disconnected
		go func() { u.opQueue <- deviceDetached(u) }()
	}
}

func connectUSBOp(u *UsbTcpBridge) func() {
	return func() {
		if u.currentState == errorTCP || u.currentState == errorUSB {
			u.log().WithFields(log.Fields{"current": u.GetStateName()}).Debug("skipping connectUSB op")
			return
		}
		u.log().Debug("Connecting usb")
		u.adapter = &UsbAdapter{Dump: false, injectedLog: u.log(), stopSignal: make(chan interface{})}
		err := u.adapter.ConnectDevice(u.device)
		if err != nil {
			u.log().Warnf("failed connecting usb %+v", err)
			u.adapter.Close()
			go func() { u.opQueue <- deviceDetached(u) }()
			return
		}
		u.adapter.StartUSBReadLoop()
		u.adapter.StartUSBWriteLoop()
		u.currentState = connectedUSB
		u.log().Debug("connected USB starting TCP")
		go func() { u.opQueue <- connectTcpOp(u) }()
	}
}

func connectTcpOp(u *UsbTcpBridge) func() {
	return func() {
		if u.currentState == detached || u.currentState == errorTCP || u.currentState == errorUSB {
			u.log().WithFields(log.Fields{"current": u.GetStateName()}).Debug("skipping connectTcpOp")
			return
		}
		u.log().Info("Starting TCP server")
		l, err := startTcp(u.port)
		if err != nil {
			u.log().WithFields(log.Fields{"port": u.port, "device": u.device.SerialNumber, "error": err}).Error("failed starting tcp server, this device is unusable now")
			u.currentState = errorTCP
			return
		}
		u.tcpServer = l
		go startHandlingConnections(l, u)
		u.log().Infof("started tcp server on port %d", u.port)
		u.currentState = online

	}
}

func processEvents(u *UsbTcpBridge) {
	for {
		u.log().Debug("waiting for operation")
		op := <-u.opQueue
		u.log().Debugf("executing operation: %s", nameOf(op))
		select {
		case <-u.done:
			u.log().Info("stopping bridge eventloop")
			u.finished <- struct{}{}
			return
		default:
			op()
		}

	}
}

func nameOf(f interface{}) string {
	v := reflect.ValueOf(f)
	if v.Kind() == reflect.Func {
		if rf := runtime.FuncForPC(v.Pointer()); rf != nil {
			return rf.Name()
		}
	}
	return v.String()
}

//Close disconnects from the usb device, shuts down the TCP socket gracefully with a timeout.
//It will always finish.
func (u *UsbTcpBridge) Close() error {
	u.log().Info("closing bridge")
	go func() {
		u.done <- struct{}{}
	}()
	go func() {
		u.opQueue <- func() {}
	}()
	select {
	case <-u.finished:
	case <-time.After(time.Second * 10):
		u.log().Error("timed out waiting for eventloop to finish")
	}

	disconnectEverything(u)()
	return nil
}

//Start connects to the USB device and starts the internal loop.
//The bridge will automatically try to re-connect whenever the device
//goes offline until Close() is called.
func (u *UsbTcpBridge) Start() error {
	go processEvents(u)
	u.opQueue <- connectUSBOp(u)
	return nil
}
func (u *UsbTcpBridge) GetStateName() string {
	_, name := GetState(u.currentState)
	return name
}

//GetState returns the current state of the device.
func GetState(currentState int) (int, string) {
	switch currentState {
	case notInitialized:
		return currentState, "notInitialized"
	case detached:
		return currentState, "detached"
	case attached:
		return currentState, "attached"
	case connecting:
		return currentState, "connecting"
	case connectedUSB:
		return currentState, "connectedUSB"
	case errorUSB:
		return currentState, "errorUSB"
	case errorTCP:
		return currentState, "errorTCP"
	case online:
		return currentState, "online"
	case disconnected:
		return currentState, "disconnected"
	default:
		panic("usb bridge was set to unknown state, this is a bug")

	}

}

func startTcp(port int) (net.Listener, error) {
	l, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", port))

	return l, err
}

func startHandlingConnections(l net.Listener, u *UsbTcpBridge) error {
	connectionAvailable := make(chan struct{}, 1)
	connectionAvailable <- struct{}{}
	tcpSender := tcpSender{}
	tcpSender.startSending(u)
	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		select {
		case <-connectionAvailable:
			tcpSender.SetConn(c)
			handleConnection(c, u, connectionAvailable)
		default:
			u.log().Warn("refusing connection")
			c.Close()

		}

	}
}

type tcpSender struct {
	tcpConn net.Conn
	mux     sync.Mutex
}

func (t *tcpSender) SetConn(conn net.Conn) {
	t.mux.Lock()
	defer t.mux.Unlock()
	t.tcpConn = conn
}

func (t *tcpSender) startSending(bridge *UsbTcpBridge) {
	bridge.log().Debug("Starting tcp sender")
	go func() {
		loop := true
		for loop {
			select {
			case packet := <-bridge.adapter.packetChannel:

				if t.tcpConn != nil {
					t.mux.Lock()
					conn := t.tcpConn
					t.mux.Unlock()
					err := WritePacketToTCP(packet, conn)
					if err != nil {
						bridge.log().Errorf("Writing to TCP failed %+v", err)

						conn.Close()
						t.SetConn(nil)

					}

				} else {
					bridge.log().Info("dropping packet, nobody connected")
				}
			case err := <-bridge.adapter.errorChannel:
				bridge.log().Errorf("bridge failed reading from usb %+v", err)

				if t.tcpConn != nil {
					t.tcpConn.Close()
				}

				go func() { bridge.opQueue <- disconnectEverything(bridge) }()
				loop = false
			}

		}
		bridge.log().Debug("finished tcp sender")
	}()
}

func handleConnection(c net.Conn, bridge *UsbTcpBridge, connectionAvailable chan struct{}) {
	bridge.log().WithFields(log.Fields{"remote": c.RemoteAddr().String()}).Info("tcp connection active")

	go func() {
		for {

			packet, err := ReadPacketFromTCP(c)
			if err != nil {
				bridge.log().Errorf("Reading From TCP failed %+v", err)
				c.Close()
				connectionAvailable <- struct{}{}
				break
			}
			err = bridge.adapter.EnqueueWrite(packet)
			if err != nil {
				bridge.log().Errorf("bridge failed writing to usb %+v", err)
				c.Close()
				go func() { bridge.opQueue <- disconnectEverything(bridge) }()
				break
			}
		}
		bridge.log().Debug("finished read from tcp write to usb")
	}()

}
