package rcon

import (
	"bytes"
	"strings"
	"testing"
)

func TestPacketRoundTrip(t *testing.T) {
	cases := []packet{
		{ID: 1, Type: typeAuth, Body: "secret"},
		{ID: 2, Type: typeExecCommand, Body: "saveworld"},
		{ID: 3, Type: typeResponseValue, Body: ""},
		{ID: -1, Type: typeAuthResponse, Body: ""},
		{ID: 99, Type: typeResponseValue, Body: strings.Repeat("x", 1024)},
	}
	for _, p := range cases {
		var buf bytes.Buffer
		if err := p.encode(&buf); err != nil {
			t.Fatalf("encode %+v: %v", p, err)
		}
		got, err := readPacket(&buf)
		if err != nil {
			t.Fatalf("readPacket: %v", err)
		}
		if got != p {
			t.Errorf("round-trip\n got: %+v\nwant: %+v", got, p)
		}
	}
}

func TestPacketEncode_RejectsOversize(t *testing.T) {
	p := packet{ID: 1, Type: typeExecCommand, Body: strings.Repeat("x", maxPayloadSize+1)}
	if err := p.encode(&bytes.Buffer{}); err == nil {
		t.Error("expected oversize rejection")
	}
}

func TestReadPacket_RejectsBadSize(t *testing.T) {
	// Payload size of 1 (well below the 10-byte minimum).
	buf := []byte{0x01, 0, 0, 0}
	if _, err := readPacket(bytes.NewReader(buf)); err == nil {
		t.Error("expected size-out-of-range rejection")
	}
}
