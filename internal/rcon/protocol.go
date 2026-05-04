package rcon

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

type packetType int32

const (
	typeResponseValue packetType = 0
	typeAuthResponse  packetType = 2
	typeExecCommand   packetType = 2
	typeAuth          packetType = 3
)

const (
	minPayloadSize = 10
	maxPayloadSize = 4096
)

type packet struct {
	ID   int32
	Type packetType
	Body string
}

func (p packet) encode(w io.Writer) error {
	body := []byte(p.Body)
	payload := 4 + 4 + len(body) + 2 // id + type + body + 2 NULs
	if payload > maxPayloadSize {
		return fmt.Errorf("rcon packet payload too large: %d > %d", payload, maxPayloadSize)
	}
	var buf bytes.Buffer
	buf.Grow(4 + payload)
	_ = binary.Write(&buf, binary.LittleEndian, int32(payload))
	_ = binary.Write(&buf, binary.LittleEndian, p.ID)
	_ = binary.Write(&buf, binary.LittleEndian, int32(p.Type))
	buf.Write(body)
	buf.WriteByte(0)
	buf.WriteByte(0)
	_, err := w.Write(buf.Bytes())
	return err
}

func readPacket(r io.Reader) (packet, error) {
	var size int32
	if err := binary.Read(r, binary.LittleEndian, &size); err != nil {
		return packet{}, err
	}
	if size < int32(minPayloadSize) || size > int32(maxPayloadSize) {
		return packet{}, fmt.Errorf("rcon packet size out of range: %d", size)
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(r, body); err != nil {
		return packet{}, err
	}
	p := packet{
		ID:   int32(binary.LittleEndian.Uint32(body[0:4])),
		Type: packetType(int32(binary.LittleEndian.Uint32(body[4:8]))),
	}
	p.Body = string(body[8 : size-2])
	return p, nil
}
