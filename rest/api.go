package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/danielpaulus/go-adb/adb"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type BridgeStatusReporter interface {
	BridgeList() []map[string]interface{}
}

func HealthHandler(s BridgeStatusReporter) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		json, err := json.Marshal(s.BridgeList())

		if err != nil {
			serverError("failed encoding json", http.StatusInternalServerError, w)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write(json)

	}
}

func DeviceResetHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serial := vars["serial"]
	log.Infof("Reset requested for: %s", serial)
	err := adb.ResetBySerial(serial)
	if err != nil {
		serverError(fmt.Sprintf("failed resetting device %s with error %v", serial, err), 500, w)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func DeviceResetVidPidHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	vid, err := strconv.Atoi(vars["vid"])
	if err != nil {
		serverError("invalid vid", http.StatusBadRequest, w)
		return
	}
	pid, err := strconv.Atoi(vars["pid"])
	if err != nil {
		serverError("invalid pid", http.StatusBadRequest, w)
		return
	}
	log.Infof("Reset requested for vid:%d  pid:%d", vid, pid)
	err = adb.ResetByVIDPID(vid, pid)
	if err != nil {
		serverError(fmt.Sprintf("failed resetting device vid:%d pid:%d with error %v", vid, pid, err), 500, w)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func serverError(message string, code int, w http.ResponseWriter) {
	json, err := json.Marshal(
		map[string]string{"error": message},
	)
	if err != nil {
		log.Warnf("error encoding json:%+v", err)
	}
	w.WriteHeader(code)
	w.Write(json)
}
