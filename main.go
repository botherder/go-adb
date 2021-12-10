package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	stdlog "log"

	"github.com/danielpaulus/go-adb/adb"
	"github.com/danielpaulus/go-adb/orchestration"
	"github.com/danielpaulus/go-adb/rest"
	"github.com/docopt/docopt-go"
	"github.com/google/gousb"
	log "github.com/sirupsen/logrus"
)

func main() {
	usage := `go-adb client v 0.01
	
	Usage:
	  go-adb single --serial=<serial> --port=<port> --vid=<vid> --pid=<pid>
	  go-adb daemon [--procperdevice]
	  go-adb listdevices

	Options:
          -h --help      Show this screen.
          

    go-adb is a drop in relpacement for adb device daemons:
	If you run it, it will try to claim all Android devices on the system and expose each on a separate TCP port.
	It exposes a small REST API to get a device list. Run 'curl localhost:16000/devices' to get the current set of devices
	known to go-adb. As long as go-adb runs, it will always put the same device on the same port.

	  go-adb single --serial=<serial> --port=<port> --vid=<vid> --pid=<pid>                     Runs go-adb only for one single device specified by serial, pid and vid. 
	  go-adb daemon [--procperdevice]                                                           Runs go-adb in daemon mode, which means it will claim every device and keep scanning for new devices. If --procperdevice is set, every device will run in its own separate process.
	  go-adb listdevices                                                                        Prints a JSON encoded devicelist. Usually used by go-adb when running with --procperdevice.                                                                   


	`
	arguments, err := docopt.ParseDoc(usage)
	if err != nil {
		log.Fatal(err)
	}

	//filter the interrupted events log from gousb, it might mess up
	//listdevices output and is in general not really useful
	stdlog.SetOutput(new(LogrusWriter))

	listdevices, _ := arguments.Bool("listdevices")
	if listdevices {
		printDeviceList()
		return

	}
	log.SetLevel(log.DebugLevel)

	log.WithFields(log.Fields{"args": os.Args, "version": GetVersion()}).Infof("starting go-adb")

	single, _ := arguments.Bool("single")
	if single {
		serial, _ := arguments.String("--serial")
		port, _ := arguments.Int("--port")
		vid, _ := arguments.Int("--vid")
		pid, _ := arguments.Int("--pid")
		device := adb.DeviceInfo{SerialNumber: serial, PID: gousb.ID(pid), VID: gousb.ID(vid)}
		log.Infof("Start in single device mode for device '%s' on port %d", serial, port)
		startBridge(device, port)
		return
	}

	daemon, _ := arguments.Bool("daemon")
	if daemon {
		log.Infof("Start in daemon mode, handling all devices")
		processPerDevice, _ := arguments.Bool("--procperdevice")
		startDaemon(processPerDevice)
		return
	}

}

const deviceBasePort = 16100
const restInterfacePort = 16000

//GetVersion reads the contents of the file version.txt and returns it.
//If the file cannot be read, it returns "could not read version"
func GetVersion() string {
	version, err := ioutil.ReadFile("version.txt")
	if err != nil {
		return "could not read version"
	}
	return string(version)
}

type LogrusWriter int

const interruptedError = "interrupted [code -10]"

func (LogrusWriter) Write(data []byte) (int, error) {
	logmessage := string(data)
	if strings.Contains(logmessage, interruptedError) {
		log.Tracef("gousb_logs:%s", logmessage)
		return len(data), nil
	}
	log.Infof("gousb_logs:%s", logmessage)
	return len(data), nil
}

func startDaemon(processPerDevice bool) {

	var deviceDetector *orchestration.DeviceDetector
	var manager *orchestration.BridgeManager
	if processPerDevice {
		execPath := executable()
		log.Info("starting device discovery in separate process")
		deviceDetector = orchestration.NewProcessDeviceDetector(execPath)
		deviceDetector.StartListening()
		log.Infof("starting device manager first device will be at port %d", deviceBasePort)
		manager = orchestration.NewSubProcessBridgeManager(execPath, deviceBasePort)
	} else {
		log.Info("starting device discovery")
		deviceDetector = orchestration.NewDeviceDetector()
		deviceDetector.StartListening()
		manager = orchestration.NewBridgeManager(deviceBasePort)
	}

	deviceDetector.AddListener(manager)
	log.Infof("starting rest api on port: %d", restInterfacePort)
	srv := rest.StartHttpServer(restInterfacePort, manager)
	log.Info("REST interface is up")

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	signal := <-c
	log.Infof("os signal:%d received, closing..", signal)

	log.Info("stopping deviceDetector..")
	deviceDetector.Close()
	log.Info("deviceDetector stopped")
	log.Info("stopping bridges..")
	manager.Close()
	log.Info("all bridges stopped")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	log.Info("shutting down REST API...")
	srv.Shutdown(ctx)
	log.Info("REST API shut down. Good bye :-) ")
}

func startBridge(device adb.DeviceInfo, port int) {
	bridge := adb.NewUsbTcpBridge(device, port)
	bridge.Start()
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	signal := <-c
	log.Infof("os signal:%d received, closing..", signal)
	bridge.Close()
	log.Info("single mode bridge is closed")
}

func executable() string {
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	return ex
}

func printDeviceList() {

	devices, err := adb.ListDevices()
	output := map[string]interface{}{"err": fmt.Sprintf("%v", err), "devicelist": devices}
	jsondevices, err := json.Marshal(output)
	if err != nil {
		panic(err)
	}
	fmt.Print(string(jsondevices))
}
