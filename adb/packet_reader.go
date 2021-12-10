package adb

import (
	"encoding/binary"
	"fmt"
	"io"

	log "github.com/sirupsen/logrus"
)

func (u *UsbAdapter) StartUSBWriteLoop() {
	u.writeChannel = make(chan Packet)
	u.writeErrorChannel = make(chan error)
	u.writeDone = make(chan interface{})

	go func() {
		u.log().Info("starting writeloop")
		for {
			select {
			case packet, ok := <-u.writeChannel:
				if !ok {
					continue
				}
				WritePacketToUSB(packet, u)
			case <-u.stopSignal:
				close(u.writeErrorChannel)
				u.writeDone <- struct{}{}
				return
			}
		}
	}()
}

func (u *UsbAdapter) EnqueueWrite(packet Packet) error {
	select {
	case err, ok := <-u.writeErrorChannel:
		if !ok {
			return io.EOF
		}
		return err
	default:
		go func() { u.writeChannel <- packet }()
	}
	return nil
}

func WritePacketToUSB(packet Packet, writer io.Writer) error {
	header := make([]byte, 24)
	binary.LittleEndian.PutUint32(header, packet.Header.CommandType)
	binary.LittleEndian.PutUint32(header[4:], packet.Header.Arg0)
	binary.LittleEndian.PutUint32(header[8:], packet.Header.Arg1)
	binary.LittleEndian.PutUint32(header[12:], packet.Header.DataLength)
	binary.LittleEndian.PutUint32(header[16:], packet.Header.Crc32)
	binary.LittleEndian.PutUint32(header[20:], packet.Header.Magic)
	_, err := writer.Write(header)
	if err != nil {
		log.Debug("failed usb sending header")
		return fmt.Errorf("Failed sending AdbPacket Header %+v to USB: %w", packet.Header, err)
	}
	payloadLength := packet.Header.DataLength
	if payloadLength == 0 {
		return nil
	}
	_, err = writer.Write(packet.Payload)
	if err != nil {
		log.Debug("failed usb sending paylod")
		return fmt.Errorf("Failed sending AdbPacket Payload %+v to USB: %w", packet.Header, err)
	}

	if payloadLength%512 == 0 {
		_, err = writer.Write(make([]byte, 0))
		if err != nil {

			return fmt.Errorf("Failed sending ZLP %w", err)
		}
	}
	return nil
}

func WritePacketToTCP(packet Packet, writer io.Writer) error {
	err := binary.Write(writer, binary.LittleEndian, packet.Header)
	if err != nil {
		return err
	}
	_, err = writer.Write(packet.Payload)

	return err
}

func ReadPacketFromTCP(reader io.Reader) (Packet, error) {
	var header PacketHeader
	err := binary.Read(reader, binary.LittleEndian, &header)
	if err != nil {
		return Packet{}, err
	}
	payload := make([]byte, header.DataLength)
	_, err = io.ReadFull(reader, payload)
	if err != nil {
		return Packet{}, err
	}
	return Packet{Header: header, Payload: payload}, err
}

func (u *UsbAdapter) StartUSBReadLoop() {
	u.packetChannel = make(chan Packet)
	u.errorChannel = make(chan error)

	go func() {
		u.log().Info("starting readloop")
		for {
			headerBytes := make([]byte, 512)
			n, err := u.Read(headerBytes)
			if err != nil {
				u.errorChannel <- err
				break
			}
			if n != 24 {
				u.log().Warn("discarding non adb header bytes")
				continue
			}
			header := PacketHeader{
				CommandType: binary.LittleEndian.Uint32(headerBytes),
				Arg0:        binary.LittleEndian.Uint32(headerBytes[4:]),
				Arg1:        binary.LittleEndian.Uint32(headerBytes[8:]),
				DataLength:  binary.LittleEndian.Uint32(headerBytes[12:]),
				Crc32:       binary.LittleEndian.Uint32(headerBytes[16:]),
				Magic:       binary.LittleEndian.Uint32(headerBytes[20:]),
			}
			if !IsValid(header.CommandType) {
				u.log().Warnf("read invalid header from USB: %x", headerBytes)
				continue
			}

			payload := make([]byte, header.DataLength)
			if header.DataLength == 0 {
				u.packetChannel <- Packet{Header: header, Payload: payload}
				continue
			}
			_, err = io.ReadFull(u, payload)
			if err != nil {
				u.errorChannel <- err
				break
			}
			u.packetChannel <- Packet{Header: header, Payload: payload}
		}
		u.log().Debug("finished usb read loop")
	}()
}
