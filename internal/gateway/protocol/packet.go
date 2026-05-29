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

// ClientPacket 是 client <-> gateway 的客户端传输信封。
// 它只关心客户端协议 ID 和原始 protobuf payload，不承担服务间 RPC 路由职责。
type ClientPacket struct {
	Type    uint16
	Payload []byte
}

// Packet 是兼容旧调用点的别名，后续新代码应优先使用 ClientPacket。
type Packet = ClientPacket

func Encode(packet ClientPacket) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, 6+len(packet.Payload)))
	_ = binary.Write(buf, binary.BigEndian, packet.Type)
	_ = binary.Write(buf, binary.BigEndian, uint32(len(packet.Payload)))
	buf.Write(packet.Payload)
	return buf.Bytes()
}

func Decode(data []byte) (ClientPacket, error) {
	if len(data) < 6 {
		return ClientPacket{}, fmt.Errorf("packet too short")
	}
	size := binary.BigEndian.Uint32(data[2:6])
	if len(data[6:]) != int(size) {
		return ClientPacket{}, fmt.Errorf("packet size mismatch")
	}
	return ClientPacket{
		Type:    binary.BigEndian.Uint16(data[:2]),
		Payload: append([]byte(nil), data[6:]...),
	}, nil
}
