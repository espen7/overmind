package network

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	// HeaderLength TCP 头部长度: Length(4) + ProtoID(4) + SeqID(4) = 12 字节
	HeaderLength = 12
	// MaxPacketSize 最大包体限制 (64KB)，防止恶意超大包攻击
	MaxPacketSize = 65535
	// WSHeaderLength WS 载荷头部长度: ProtoID(4) + SeqID(4) = 8 字节
	WSHeaderLength = 8
)

// Packet 代表解包后的网络报文
type Packet struct {
	ProtoID int32  // 协议ID
	SeqID   int32  // 序列号
	Payload []byte // 二进制协议载荷 (Protobuf)
}

// ==================== TCP 流封包与解包 ====================

// PackTCP 将协议序列化为标准的 Length-Header 二进制流
func PackTCP(protoID int32, seqID int32, payload []byte) ([]byte, error) {
	totalLen := uint32(HeaderLength + len(payload))
	if totalLen > MaxPacketSize {
		return nil, errors.New("packet size exceeds limit")
	}

	buf := make([]byte, totalLen)
	binary.BigEndian.PutUint32(buf[0:4], totalLen)
	binary.BigEndian.PutUint32(buf[4:8], uint32(protoID))
	binary.BigEndian.PutUint32(buf[8:12], uint32(seqID))
	copy(buf[12:], payload)
	return buf, nil
}

// ReadPacketTCP 从输入流中读取并解析一个标准的 TCP 报文
func ReadPacketTCP(r io.Reader) (*Packet, error) {
	header := make([]byte, HeaderLength)
	_, err := io.ReadFull(r, header)
	if err != nil {
		return nil, err
	}

	totalLen := binary.BigEndian.Uint32(header[0:4])
	if totalLen < HeaderLength || totalLen > MaxPacketSize {
		return nil, errors.New("invalid packet length")
	}

	protoID := int32(binary.BigEndian.Uint32(header[4:8]))
	seqID := int32(binary.BigEndian.Uint32(header[8:12]))

	payloadLen := totalLen - HeaderLength
	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		_, err = io.ReadFull(r, payload)
		if err != nil {
			return nil, err
		}
	}

	return &Packet{
		ProtoID: protoID,
		SeqID:   seqID,
		Payload: payload,
	}, nil
}

// ==================== WebSocket 帧封包与解包 ====================

// PackWS 将协议序列化为 WebSocket 二进制帧负载 (去掉了 Length，因为 WS 帧自带长度)
func PackWS(protoID int32, seqID int32, payload []byte) ([]byte, error) {
	totalLen := WSHeaderLength + len(payload)
	if totalLen > MaxPacketSize {
		return nil, errors.New("packet size exceeds limit")
	}

	buf := make([]byte, totalLen)
	binary.BigEndian.PutUint32(buf[0:4], uint32(protoID))
	binary.BigEndian.PutUint32(buf[4:8], uint32(seqID))
	copy(buf[8:], payload)
	return buf, nil
}

// UnpackWS 将 WebSocket 接收到的二进制帧负载解包
func UnpackWS(data []byte) (*Packet, error) {
	if len(data) < WSHeaderLength {
		return nil, errors.New("websocket packet too short")
	}
	if len(data) > MaxPacketSize {
		return nil, errors.New("websocket packet size exceeds limit")
	}

	protoID := int32(binary.BigEndian.Uint32(data[0:4]))
	seqID := int32(binary.BigEndian.Uint32(data[4:8]))
	payload := data[8:]

	return &Packet{
		ProtoID: protoID,
		SeqID:   seqID,
		Payload: payload,
	}, nil
}
