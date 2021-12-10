package adb

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	log "github.com/sirupsen/logrus"
)

type subProcessBridge struct {
	device       DeviceInfo
	port         int
	done         chan struct{}
	finished     chan struct{}
	cmd          *exec.Cmd
	goadbPath    string
	currentState int
}

//NewSubProcessBridge creates a Bridge that will start the device usb-tcp bridge
//in a separate go-adb process using the go-adb single command.
//the process will be automatically restarted should it crash or shutdown.
//On Close we send a SIGTERM to the childprocess.
//device is the DeviceInfo for the device we need to bridge, port is the TCP port on which
//the device will be available and goadbpath is the go-adb binary to start.
func NewSubProcessBridge(device DeviceInfo, port int, goadbPath string) *subProcessBridge {
	return &subProcessBridge{
		device:       device,
		port:         port,
		done:         make(chan struct{}),
		finished:     make(chan struct{}),
		goadbPath:    goadbPath,
		currentState: detached,
	}
}

//GetSerialNumber returns the serial usb number of the device this bridge is responsible for.
func (s *subProcessBridge) GetSerialNumber() string {
	return s.device.SerialNumber
}

//Close sends a SIGTERM to the childprocess and waits for it to shut down.
//Currently there is no timeout here, go-adb is expected to always shutdown.
func (s *subProcessBridge) Close() error {
	//https://bigkevmcd.github.io/go/pgrp/context/2019/02/19/terminating-processes-in-go.html
	go func() { s.done <- struct{}{} }()
	log.Info("sending sigterm")
	syscall.Kill(s.cmd.Process.Pid, syscall.SIGTERM)
	<-s.finished
	log.Info("closing bridge")
	return nil
}

//Start launches a new go-adb process for the device, make sure to only call once.
func (s *subProcessBridge) Start() error {
	go func() {
		for {
			log.Info("starting bridge process")
			s.cmd = exec.Command(s.goadbPath,
				"single", fmt.Sprintf("--serial=%s", s.device.SerialNumber), fmt.Sprintf("--port=%d", s.port),
				fmt.Sprintf("--vid=%d", s.device.VID), fmt.Sprintf("--pid=%d", s.device.PID),
			)
			s.cmd.Stdout = os.Stdout
			s.cmd.Stderr = os.Stderr
			err := s.cmd.Start()
			if err != nil {
				log.Error("failed starting process:" + err.Error())
				continue
			}
			log.Info("waiting bridge process to complete")
			s.currentState = online
			err = s.cmd.Wait()
			if err != nil {
				log.Warnf("bridge process failed with:%+v", err)
			}
			s.currentState = detached
			log.Info("bridge process done")
			select {
			case <-s.done:
				s.finished <- struct{}{}
				return
			default:
			}
		}
	}()
	return nil
}

//Basic implementation
func (s *subProcessBridge) GetStateName() string {
	_, name := GetState(s.currentState)
	return name
}
