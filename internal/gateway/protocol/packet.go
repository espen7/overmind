package protocol

import (
	"bytes"
	"crypto/aes"
	"encoding/binary"
	"errors"
	"overmind/internal/kit/crc16" // We need to implement this
)

// Packet Format (Binary WS Frame):
// [Encrypted Payload (N-2 bytes)] + [CRC16 (2 bytes)]
//
// Encrypted Payload Decrypted:
// [MsgType (2 bytes)] + [MsgNo (4 bytes)] + [Protobuf Body (N-6 bytes)]

const (
	HeaderSize = 6 // MsgType(2) + MsgNo(4)
	CRCSize    = 2
)

var (
	ErrInvalidCRC       = errors.New("invalid crc")
	ErrPacketTooShort   = errors.New("packet too short")
	ErrDecryptionFailed = errors.New("decryption failed")
)

// Packet represents a decrypted message ready for processing
type Packet struct {
	MsgType int32
	MsgNo   int32
	Body    []byte
}

// Unpack decodes a raw frame from the client
// 1. Verify CRC
// 2. Decrypt Payload
// 3. Parse Header
func Unpack(raw []byte, aesKey []byte) (*Packet, error) {
	if len(raw) < CRCSize+HeaderSize {
		return nil, ErrPacketTooShort
	}

	// 1. CRC Check
	payloadLen := len(raw) - CRCSize
	payload := raw[:payloadLen]
	receivedCRC := binary.BigEndian.Uint16(raw[payloadLen:])

	calculatedCRC := crc16.Checksum(payload)
	if calculatedCRC != receivedCRC {
		return nil, ErrInvalidCRC
	}

	// 2. Decrypt (AES ECB)
	decrypted, err := AesDecrypt(payload, aesKey)
	if err != nil {
		return nil, err
	}

	// 3. Parse Header
	// MsgType (Shoet -> int32 internally)
	msgType := int32(binary.BigEndian.Uint16(decrypted[0:2]))
	// MsgNo (Int)
	msgNo := int32(binary.BigEndian.Uint32(decrypted[2:6]))
	// Body
	body := decrypted[6:]

	return &Packet{
		MsgType: msgType,
		MsgNo:   msgNo,
		Body:    body,
	}, nil
}

// Pack encodes a message to be sent to the client
// Format: [Header(8 bytes)] + [Payload(N)] + [CRC(2)]
// Header: MsgType(2) + MsgNo(4) + Rt(2)
// Note: Depending on logic, Body might be encrypted. Here we assume Raw Body.
func Pack(msgType int32, msgNo int32, rt int32, body []byte) ([]byte, error) {
	// Total Buffer = Header(8) + Body + CRC(2)
	buf := new(bytes.Buffer)

	// Header
	binary.Write(buf, binary.BigEndian, int16(msgType))
	binary.Write(buf, binary.BigEndian, int32(msgNo))
	binary.Write(buf, binary.BigEndian, int16(rt))

	// Body
	buf.Write(body)

	// Calculate CRC for (Header + Body)
	content := buf.Bytes()
	crc := crc16.Checksum(content)

	// Append CRC
	finalBuf := make([]byte, len(content)+2)
	copy(finalBuf, content)
	binary.BigEndian.PutUint16(finalBuf[len(content):], crc)

	return finalBuf, nil
}

// AesDecrypt decrypts using AES-ECB-PKCS5
// Note: ECB is generally not recommended but required by protocol spec.
func AesDecrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	if len(ciphertext)%block.BlockSize() != 0 {
		return nil, errors.New("ciphertext is not a multiple of the block size")
	}

	decrypted := make([]byte, len(ciphertext))
	blockSize := block.BlockSize()

	// Handler ECB manually (Go doesn't support ECB out of the box for security reasons)
	for bs, be := 0, blockSize; bs < len(ciphertext); bs, be = bs+blockSize, be+blockSize {
		block.Decrypt(decrypted[bs:be], ciphertext[bs:be])
	}

	// PKCS5 Unpadding
	return pkcs5Unpadding(decrypted)
}

func pkcs5Unpadding(src []byte) ([]byte, error) {
	length := len(src)
	unpadding := int(src[length-1])
	if unpadding > length {
		return nil, errors.New("unpadding error")
	}
	return src[:(length - unpadding)], nil
}
