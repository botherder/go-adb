package adb_test

import (
	"testing"

	"github.com/danielpaulus/go-adb/adb"
	"github.com/stretchr/testify/assert"
)

func TestIsValid(t *testing.T) {
	assert.True(t, adb.IsValid(adb.Auth))
	assert.True(t, adb.IsValid(adb.Cnxn))
	assert.True(t, adb.IsValid(adb.Clse))
	assert.True(t, adb.IsValid(adb.Okay))
	assert.True(t, adb.IsValid(adb.Open))
	assert.True(t, adb.IsValid(adb.Sync))
	assert.True(t, adb.IsValid(adb.Wrte))

	assert.False(t, adb.IsValid(0x987))
}
