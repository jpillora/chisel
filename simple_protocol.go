package chisel

import (
	"encoding/binary"
	"net"
)

//simple protocol [2 bytes of size, size bytes of data]

func SizeRead(c net.Conn) []byte {
	sizeb := make([]byte, 2)
	c.Read(sizeb)
	size := binary.BigEndian.Uint16(sizeb)
	datab := make([]byte, size)
	c.Read(datab)
	return datab
}

func SizeWrite(c net.Conn, data []byte) {
	size := len(data)
	sizeb := make([]byte, 2)
	binary.BigEndian.PutUint16(sizeb, uint16(size))
	c.Write(sizeb)
	c.Write(data)
	return
}
