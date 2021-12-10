package orchestration_test

import (
	"encoding/json"
	"testing"

	"github.com/danielpaulus/go-adb/adb"
	"github.com/danielpaulus/go-adb/orchestration"
	"github.com/stretchr/testify/assert"
)

var info adb.DeviceInfo = adb.DeviceInfo{SerialNumber: "test", ProductName: "test", VID: 5, PID: 6}
var info2 adb.DeviceInfo = adb.DeviceInfo{SerialNumber: "test2", ProductName: "test2", VID: 5, PID: 6}

const basePort = 60000

func TestBridgeManagerInitialList(t *testing.T) {
	man := orchestration.NewBridgeManager(basePort)

	man.InitialList([]adb.DeviceInfo{info, info2})
	const expected = `[{"port":60000,"serial":"test","state":"notInitialized"},{"port":60001,"serial":"test2","state":"notInitialized"}]`
	actual := tojson(man.BridgeList(), t)
	assert.Equal(t, expected, actual)
	err := man.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestBridgeManagerAddDevice(t *testing.T) {
	man := orchestration.NewBridgeManager(basePort)

	man.InitialList([]adb.DeviceInfo{})

	expected := `[]`
	actual := tojson(man.BridgeList(), t)
	assert.Equal(t, expected, actual)

	man.DeviceAdded(info)
	man.DeviceAdded(info2)
	man.DeviceAdded(info2)
	expected = `[{"port":60000,"serial":"test","state":"notInitialized"},{"port":60001,"serial":"test2","state":"notInitialized"}]`
	actual = tojson(man.BridgeList(), t)
	assert.Equal(t, expected, actual)

	err := man.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func tojson(obj interface{}, t *testing.T) string {
	json, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	return string(json)
}
