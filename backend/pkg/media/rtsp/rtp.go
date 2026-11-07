package rtsp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

var (
	ErrInvalidRTPHeader = errors.New("invalid RTP header")
	ErrMagicByteMismatch = errors.New("expected '$' magic byte for interleaved RTP frame")
)

// RTPPacket represents a parsed Real-time Transport Protocol packet
type RTPPacket struct {
	Channel        int
	Version        int
	Padding        bool
	Extension      bool
	Marker         bool
	PayloadType    int
	SequenceNumber uint16
	Timestamp      uint32
	SSRC           uint32
	Payload        []byte
}

// ReadInterleavedPacket reads a single TCP-interleaved RTP/RTCP packet from an RTSP connection
func ReadInterleavedPacket(r io.Reader) (*RTPPacket, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	if header[0] != '$' {
		return nil, fmt.Errorf("%w: got 0x%02X", ErrMagicByteMismatch, header[0])
	}

	channel := int(header[1])
	length := int(binary.BigEndian.Uint16(header[2:4]))

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}

	// If length is less than minimal 12 bytes of RTP header, return raw packet
	if length < 12 {
		return &RTPPacket{
			Channel: channel,
			Payload: payload,
		}, nil
	}

	// Parse RFC 3550 RTP Header
	v := int(payload[0] >> 6)
	p := (payload[0] & 0x20) != 0
	x := (payload[0] & 0x10) != 0
	cc := int(payload[0] & 0x0F)
	m := (payload[1] & 0x80) != 0
	pt := int(payload[1] & 0x7F)
	seq := binary.BigEndian.Uint16(payload[2:4])
	ts := binary.BigEndian.Uint32(payload[4:8])
	ssrc := binary.BigEndian.Uint32(payload[8:12])

	headerOffset := 12 + cc*4
	if length < headerOffset {
		return nil, ErrInvalidRTPHeader
	}

	// Handle optional header extension
	if x && length >= headerOffset+4 {
		extLen := int(binary.BigEndian.Uint16(payload[headerOffset+2:headerOffset+4])) * 4
		headerOffset += 4 + extLen
		if length < headerOffset {
			return nil, ErrInvalidRTPHeader
		}
	}

	return &RTPPacket{
		Channel:        channel,
		Version:        v,
		Padding:        p,
		Extension:      x,
		Marker:         m,
		PayloadType:    pt,
		SequenceNumber: seq,
		Timestamp:      ts,
		SSRC:           ssrc,
		Payload:        payload[headerOffset:],
	}, nil
}
