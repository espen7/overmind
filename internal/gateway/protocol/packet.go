package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	MessageTypeLoginRequest    uint16 = 1001
	MessageTypeLoginResponse   uint16 = 1002
	MessageTypeEnterScene      uint16 = 2001
	MessageTypeSceneSnapshot   uint16 = 2002
	MessageTypeMoveRequest     uint16 = 2003
	MessageTypeMoveBroadcast   uint16 = 2004
	MessageTypeAttackRequest   uint16 = 2005
	MessageTypeCombatBroadcast uint16 = 2006
	MessageTypeErrorResponse   uint16 = 9000
)

type Packet struct {
	Type    uint16
	Payload []byte
}

func Encode(packet Packet) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, 6+len(packet.Payload)))
	_ = binary.Write(buf, binary.BigEndian, packet.Type)
	_ = binary.Write(buf, binary.BigEndian, uint32(len(packet.Payload)))
	buf.Write(packet.Payload)
	return buf.Bytes()
}

func Decode(data []byte) (Packet, error) {
	if len(data) < 6 {
		return Packet{}, fmt.Errorf("packet too short")
	}
	size := binary.BigEndian.Uint32(data[2:6])
	if len(data[6:]) != int(size) {
		return Packet{}, fmt.Errorf("packet size mismatch")
	}
	return Packet{
		Type:    binary.BigEndian.Uint16(data[:2]),
		Payload: append([]byte(nil), data[6:]...),
	}, nil
}
