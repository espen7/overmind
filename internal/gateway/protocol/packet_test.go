package protocol

import "testing"

func TestPacketRoundTrip(t *testing.T) {
	packet := Packet{Type: 1001, Payload: []byte("hello")}
	encoded := Encode(packet)
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("decode returned error: %v", err)
	}
	if decoded.Type != packet.Type {
		t.Fatalf("unexpected type: got %d want %d", decoded.Type, packet.Type)
	}
	if string(decoded.Payload) != "hello" {
		t.Fatalf("unexpected payload: %q", decoded.Payload)
	}
}
