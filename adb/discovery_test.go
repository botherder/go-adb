package adb_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/danielpaulus/go-adb/adb"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestDiscovery(t *testing.T) {
	adbDevicesCommand := exec.Command("adb", "devices", "-l")
	output, err := adbDevicesCommand.Output()
	if err != nil {
		log.Fatalf("ADB invocation failed: %+v", err)
	}
	devicesOutput := string(output)
	log.Infof("adb devices:%s", devicesOutput)
	if !strings.Contains(devicesOutput, "device") {
		log.Fatal("adb devices did not contain one usb device.")
	}

	devices, err := adb.ListDevices()
	if assert.NoError(t, err) {
		//disable until real device test runners are available
		//assert.Greater(t, len(devices), 0)
		for _, info := range devices {
			assert.Contains(t, devicesOutput, info.SerialNumber)
		}
	}
}
