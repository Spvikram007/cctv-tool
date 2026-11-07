package rtsp

import (
	"strconv"
	"strings"
)

// MediaTrack represents a parsed media stream description (video or audio)
type MediaTrack struct {
	Type        string // "video" or "audio"
	Port        int
	Protocol    string // e.g. "RTP/AVP"
	PayloadType int
	Codec       string // "H264", "H265", "JPEG", "PCMU", "PCMA", "AAC", etc.
	ClockRate   int    // e.g. 90000
	ControlURL  string // relative or absolute control path
	Width       int
	Height      int
	FPS         int
	FmtpParams  map[string]string
}

// SDPInfo contains parsed Session Description Protocol metadata
type SDPInfo struct {
	SessionName string
	SessionInfo string
	Origin      string
	ControlURL  string
	Tracks      []MediaTrack
}

// VideoTrack returns the primary video track, or nil if none found
func (s *SDPInfo) VideoTrack() *MediaTrack {
	for i := range s.Tracks {
		if s.Tracks[i].Type == "video" {
			return &s.Tracks[i]
		}
	}
	return nil
}

// AudioTrack returns the primary audio track, or nil if none found
func (s *SDPInfo) AudioTrack() *MediaTrack {
	for i := range s.Tracks {
		if s.Tracks[i].Type == "audio" {
			return &s.Tracks[i]
		}
	}
	return nil
}

// ParseSDP parses raw SDP text into structured SDPInfo
func ParseSDP(raw string) *SDPInfo {
	sdp := &SDPInfo{
		Tracks: make([]MediaTrack, 0),
	}

	lines := strings.Split(raw, "\n")
	var currentTrack *MediaTrack

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 2 || line[1] != '=' {
			continue
		}

		key := line[0]
		val := strings.TrimSpace(line[2:])

		switch key {
		case 's':
			sdp.SessionName = val
		case 'i':
			sdp.SessionInfo = val
		case 'o':
			sdp.Origin = val

		case 'm':
			// New media track: e.g. "video 0 RTP/AVP 96"
			if currentTrack != nil {
				sdp.Tracks = append(sdp.Tracks, *currentTrack)
			}
			parts := strings.Fields(val)
			if len(parts) >= 3 {
				port, _ := strconv.Atoi(parts[1])
				pt, _ := strconv.Atoi(parts[len(parts)-1])
				currentTrack = &MediaTrack{
					Type:        parts[0],
					Port:        port,
					Protocol:    parts[2],
					PayloadType: pt,
					FmtpParams:  make(map[string]string),
				}
			}

		case 'a':
			// Attribute line
			if currentTrack == nil {
				// Session-level attribute
				if strings.HasPrefix(val, "control:") {
					sdp.ControlURL = strings.TrimPrefix(val, "control:")
				}
				continue
			}

			// Media-level attribute
			if strings.HasPrefix(val, "control:") {
				currentTrack.ControlURL = strings.TrimPrefix(val, "control:")
			} else if strings.HasPrefix(val, "rtpmap:") {
				// e.g. "96 H264/90000" or "97 H265/90000"
				rtpmap := strings.TrimPrefix(val, "rtpmap:")
				parts := strings.Fields(rtpmap)
				if len(parts) >= 2 {
					codecParts := strings.Split(parts[1], "/")
					currentTrack.Codec = strings.ToUpper(codecParts[0])
					if len(codecParts) > 1 {
						rate, _ := strconv.Atoi(codecParts[1])
						currentTrack.ClockRate = rate
					}
				}
			} else if strings.HasPrefix(val, "fmtp:") {
				// e.g. "96 packetization-mode=1;sprop-parameter-sets=..."
				fmtp := strings.TrimPrefix(val, "fmtp:")
				parts := strings.SplitN(fmtp, " ", 2)
				if len(parts) == 2 {
					for _, param := range strings.Split(parts[1], ";") {
						param = strings.TrimSpace(param)
						if kv := strings.SplitN(param, "=", 2); len(kv) == 2 {
							currentTrack.FmtpParams[strings.ToLower(kv[0])] = kv[1]
						}
					}
				}
			} else if strings.HasPrefix(val, "imageattr:") {
				// Parse dimension hints e.g. "[x=1920,y=1080]"
				if idx := strings.Index(val, "x="); idx != -1 {
					wStr := val[idx+2:]
					if endIdx := strings.IndexAny(wStr, ", ]"); endIdx != -1 {
						w, _ := strconv.Atoi(wStr[:endIdx])
						currentTrack.Width = w
					}
				}
				if idx := strings.Index(val, "y="); idx != -1 {
					hStr := val[idx+2:]
					if endIdx := strings.IndexAny(hStr, ", ]"); endIdx != -1 {
						h, _ := strconv.Atoi(hStr[:endIdx])
						currentTrack.Height = h
					}
				}
			}
		}
	}

	if currentTrack != nil {
		sdp.Tracks = append(sdp.Tracks, *currentTrack)
	}

	// Normalise payload types if codec name was not in rtpmap
	for i := range sdp.Tracks {
		t := &sdp.Tracks[i]
		if t.Codec == "" {
			switch t.PayloadType {
			case 0:
				t.Codec = "PCMU"
				t.ClockRate = 8000
			case 8:
				t.Codec = "PCMA"
				t.ClockRate = 8000
			case 26:
				t.Codec = "JPEG"
				t.ClockRate = 90000
			default:
				if t.Type == "video" {
					t.Codec = "H264"
					t.ClockRate = 90000
				}
			}
		}
	}

	return sdp
}
