package rtsp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestParseSDP(t *testing.T) {
	sdpRaw := `v=0
o=- 1609459200 1609459200 IN IP4 192.168.1.100
s=HIKVISION Live Stream
c=IN IP4 0.0.0.0
t=0 0
a=control:*
m=video 0 RTP/AVP 96
a=control:trackID=1
a=rtpmap:96 H264/90000
a=fmtp:96 packetization-mode=1;profile-level-id=4D401F;sprop-parameter-sets=Z01AH42NQDwBE/LgAQ==,aO48gA==
a=imageattr:96 [x=1920,y=1080]
m=audio 0 RTP/AVP 8
a=control:trackID=2
a=rtpmap:8 PCMA/8000
`

	sdp := ParseSDP(sdpRaw)
	if sdp.SessionName != "HIKVISION Live Stream" {
		t.Errorf("expected session name 'HIKVISION Live Stream', got '%s'", sdp.SessionName)
	}

	vt := sdp.VideoTrack()
	if vt == nil {
		t.Fatalf("expected video track, got nil")
	}
	if vt.Codec != "H264" {
		t.Errorf("expected H264 codec, got %s", vt.Codec)
	}
	if vt.ClockRate != 90000 {
		t.Errorf("expected clock rate 90000, got %d", vt.ClockRate)
	}
	if vt.Width != 1920 || vt.Height != 1080 {
		t.Errorf("expected 1920x1080, got %dx%d", vt.Width, vt.Height)
	}
	if vt.ControlURL != "trackID=1" {
		t.Errorf("expected control trackID=1, got %s", vt.ControlURL)
	}

	at := sdp.AudioTrack()
	if at == nil {
		t.Fatalf("expected audio track, got nil")
	}
	if at.Codec != "PCMA" {
		t.Errorf("expected PCMA codec, got %s", at.Codec)
	}
	if at.ClockRate != 8000 {
		t.Errorf("expected clock rate 8000, got %d", at.ClockRate)
	}
}

func TestReadInterleavedPacket(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteByte('$')
	buf.WriteByte(0) // channel 0

	// Create 12-byte minimal RTP packet
	rtpHeader := make([]byte, 12)
	rtpHeader[0] = 0x80 // v=2
	rtpHeader[1] = 0xE0 // m=1, pt=96
	binary.BigEndian.PutUint16(rtpHeader[2:4], 1024)     // seq
	binary.BigEndian.PutUint32(rtpHeader[4:8], 90000)    // ts
	binary.BigEndian.PutUint32(rtpHeader[8:12], 0x1234)  // ssrc

	payload := []byte("H264_FRAME_DATA_12345678")
	totalLen := len(rtpHeader) + len(payload)

	lengthBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(lengthBytes, uint16(totalLen))
	buf.Write(lengthBytes)
	buf.Write(rtpHeader)
	buf.Write(payload)

	pkt, err := ReadInterleavedPacket(&buf)
	if err != nil {
		t.Fatalf("ReadInterleavedPacket failed: %v", err)
	}

	if pkt.Channel != 0 {
		t.Errorf("expected channel 0, got %d", pkt.Channel)
	}
	if !pkt.Marker {
		t.Errorf("expected marker=true")
	}
	if pkt.SequenceNumber != 1024 {
		t.Errorf("expected sequence number 1024, got %d", pkt.SequenceNumber)
	}
	if pkt.Timestamp != 90000 {
		t.Errorf("expected timestamp 90000, got %d", pkt.Timestamp)
	}
	if string(pkt.Payload) != string(payload) {
		t.Errorf("expected payload '%s', got '%s'", payload, pkt.Payload)
	}
}

func TestRTSPClientFullLifecycle(t *testing.T) {
	// Spin up in-memory mock RTSP TCP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind listener: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			buf := make([]byte, 2048)
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			reqStr := string(buf[:n])

			if strings.HasPrefix(reqStr, "OPTIONS") {
				resp := "RTSP/1.0 200 OK\r\nCSeq: 1\r\nPublic: OPTIONS, DESCRIBE, SETUP, PLAY, TEARDOWN\r\n\r\n"
				conn.Write([]byte(resp))
			} else if strings.HasPrefix(reqStr, "DESCRIBE") {
				sdpBody := "v=0\r\ns=Test Stream\r\nm=video 0 RTP/AVP 96\r\na=control:track1\r\na=rtpmap:96 H264/90000\r\n"
				resp := fmt.Sprintf("RTSP/1.0 200 OK\r\nCSeq: 2\r\nContent-Type: application/sdp\r\nContent-Length: %d\r\n\r\n%s", len(sdpBody), sdpBody)
				conn.Write([]byte(resp))
			} else if strings.HasPrefix(reqStr, "SETUP") {
				resp := "RTSP/1.0 200 OK\r\nCSeq: 3\r\nSession: S12345678\r\nTransport: RTP/AVP/TCP;unicast;interleaved=0-1\r\n\r\n"
				conn.Write([]byte(resp))
			} else if strings.HasPrefix(reqStr, "PLAY") {
				resp := "RTSP/1.0 200 OK\r\nCSeq: 4\r\nSession: S12345678\r\n\r\n"
				conn.Write([]byte(resp))

				// Send 1 interleaved packet
				var pkt bytes.Buffer
				pkt.WriteByte('$')
				pkt.WriteByte(0)
				rtpHdr := make([]byte, 12)
				rtpHdr[0] = 0x80
				binary.BigEndian.PutUint16(rtpHdr[2:4], 1)
				data := []byte("SAMPLE_NALU")
				length := uint16(len(rtpHdr) + len(data))
				binary.Write(&pkt, binary.BigEndian, length)
				pkt.Write(rtpHdr)
				pkt.Write(data)
				conn.Write(pkt.Bytes())
			} else if strings.HasPrefix(reqStr, "TEARDOWN") {
				resp := "RTSP/1.0 200 OK\r\nCSeq: 5\r\nSession: S12345678\r\n\r\n"
				conn.Write([]byte(resp))
				return
			}
		}
	}()

	client := NewClient(fmt.Sprintf("rtsp://127.0.0.1:%d/live/ch0", port), "admin", "Admin123", 2*time.Second)
	if err := client.Connect(); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	// 1. OPTIONS
	methods, err := client.Options()
	if err != nil {
		t.Fatalf("Options failed: %v", err)
	}
	if len(methods) < 4 {
		t.Errorf("expected at least 4 methods in OPTIONS, got %v", methods)
	}

	// 2. DESCRIBE
	sdp, err := client.Describe()
	if err != nil {
		t.Fatalf("Describe failed: %v", err)
	}
	if sdp.VideoTrack() == nil || sdp.VideoTrack().Codec != "H264" {
		t.Errorf("expected H264 video track")
	}

	// 3. SETUP
	_, err = client.Setup(sdp.VideoTrack().ControlURL, 0)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	if client.SessionID() != "S12345678" {
		t.Errorf("expected session S12345678, got %s", client.SessionID())
	}

	// 4. PLAY
	if err := client.Play(); err != nil {
		t.Fatalf("Play failed: %v", err)
	}

	// 5. ReadPacket
	pkt, err := client.ReadPacket()
	if err != nil && err != io.EOF {
		t.Fatalf("ReadPacket failed: %v", err)
	}
	if pkt != nil && string(pkt.Payload) != "SAMPLE_NALU" {
		t.Errorf("expected payload SAMPLE_NALU, got: %s", pkt.Payload)
	}

	// 6. TEARDOWN
	if err := client.Teardown(); err != nil {
		t.Fatalf("Teardown failed: %v", err)
	}
}
