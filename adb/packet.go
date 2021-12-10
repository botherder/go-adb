package adb

//the 7 different adb packet/command types
const (
	Auth uint32 = 0x48545541
	Cnxn uint32 = 0x4e584e43
	Clse uint32 = 0x45534c43
	Okay uint32 = 0x59414b4f
	Open uint32 = 0x4e45504f
	Sync uint32 = 0x434e5953
	Wrte uint32 = 0x45545257
)

//PacketHeader contains the 24 bytes header for a Packet
//The first 4 bytes must be one of the commands above
// DataLength indicates how long the payload of the packet will be
type PacketHeader struct {
	CommandType uint32
	Arg0        uint32
	Arg1        uint32
	DataLength  uint32
	Crc32       uint32
	Magic       uint32
}

//IsValid checks if a given uint32 is one of the valid adb command signature uint32
func IsValid(commandType uint32) bool {
	return commandType == Auth ||
		commandType == Cnxn ||
		commandType == Clse ||
		commandType == Okay ||
		commandType == Open ||
		commandType == Sync ||
		commandType == Wrte
}

//Packet is one adb data packet that will be sent over
//USB to the device
type Packet struct {
	Header  PacketHeader
	Payload []byte
}
