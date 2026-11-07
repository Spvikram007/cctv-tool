package rtsp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cctv-sentinel/pkg/adapters/httpauth"
)

var (
	ErrRTSPUnauthorized   = errors.New("rtsp: authentication required / invalid credentials")
	ErrRTSPNotFound       = errors.New("rtsp: stream uri not found (404)")
	ErrRTSPSessionInvalid = errors.New("rtsp: session expired or invalid (454)")
	ErrRTSPTransportFail  = errors.New("rtsp: transport negotiation unsupported (461)")
	ErrRTSPNotConnected   = errors.New("rtsp: client is not connected")
)

// Response represents a parsed RTSP response
type Response struct {
	StatusCode int
	StatusText string
	Proto      string
	Headers    map[string]string
	Body       []byte
}

// Client implements a robust RTSP 1.0 client with Digest auth and interleaved TCP streaming
type Client struct {
	TargetURL    string
	Username     string
	Password     string
	Timeout      time.Duration

	mu           sync.Mutex
	conn         net.Conn
	reader       *bufio.Reader
	cseq         uint32
	sessionID    string
	authChallenge *httpauth.Challenge
	nonceCount   uint32
	connected    bool
	sdpInfo      *SDPInfo
}

// NewClient creates a new RTSP client
func NewClient(rawURL, username, password string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		TargetURL: rawURL,
		Username:  username,
		Password:  password,
		Timeout:   timeout,
	}
}

// Connect dials the RTSP server
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected && c.conn != nil {
		return nil
	}

	u, err := url.Parse(c.TargetURL)
	if err != nil {
		return fmt.Errorf("invalid RTSP URL: %w", err)
	}

	host := u.Host
	if !strings.Contains(host, ":") {
		host = host + ":554"
	}

	d := net.Dialer{Timeout: c.Timeout}
	conn, err := d.Dial("tcp", host)
	if err != nil {
		return fmt.Errorf("failed to dial RTSP host %s: %w", host, err)
	}

	c.conn = conn
	c.reader = bufio.NewReaderSize(conn, 65536)
	c.connected = true
	c.cseq = 0

	return nil
}

// Close gracefully closes the RTSP connection
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.connected = false
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

// sendRequestRaw writes an RTSP request and reads the response
func (c *Client) sendRequest(method, reqURI string, extraHeaders map[string]string, body []byte) (*Response, error) {
	if !c.connected || c.conn == nil {
		return nil, ErrRTSPNotConnected
	}

	cseq := atomic.AddUint32(&c.cseq, 1)

	var req strings.Builder
	req.WriteString(fmt.Sprintf("%s %s RTSP/1.0\r\n", method, reqURI))
	req.WriteString(fmt.Sprintf("CSeq: %d\r\n", cseq))
	req.WriteString("User-Agent: CCTV-Sentinel-Universal/1.0\r\n")

	if c.sessionID != "" && method != "OPTIONS" && method != "DESCRIBE" {
		req.WriteString(fmt.Sprintf("Session: %s\r\n", c.sessionID))
	}

	// Add Authorization header if challenge is cached
	if c.authChallenge != nil && c.Username != "" {
		nc := atomic.AddUint32(&c.nonceCount, 1)
		cnonce := httpauth.GenerateCNonce()
		authHdr, err := httpauth.ComputeDigestResponse(c.authChallenge, c.Username, c.Password, method, reqURI, nc, cnonce)
		if err == nil {
			req.WriteString(fmt.Sprintf("Authorization: %s\r\n", authHdr))
		}
	}

	for k, v := range extraHeaders {
		req.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}

	if len(body) > 0 {
		req.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(body)))
	}
	req.WriteString("\r\n")

	if len(body) > 0 {
		req.Write(body)
	}

	// Set deadline for request/response
	_ = c.conn.SetDeadline(time.Now().Add(c.Timeout))

	if _, err := io.WriteString(c.conn, req.String()); err != nil {
		c.connected = false
		return nil, err
	}

	// Read response status line
	statusLine, err := c.reader.ReadString('\n')
	if err != nil {
		c.connected = false
		return nil, err
	}
	statusLine = strings.TrimSpace(statusLine)

	// In interleaved mode, if dollar signs arrive, skip until status line
	for len(statusLine) > 0 && statusLine[0] == '$' {
		// Interleaved packet arrived before response status
		if len(statusLine) < 4 {
			break
		}
		length := int(statusLine[2])<<8 | int(statusLine[3])
		_, _ = c.reader.Discard(length)
		statusLine, err = c.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		statusLine = strings.TrimSpace(statusLine)
	}

	statusParts := strings.SplitN(statusLine, " ", 3)
	if len(statusParts) < 2 {
		return nil, fmt.Errorf("malformed RTSP status line: %s", statusLine)
	}

	statusCode, _ := strconv.Atoi(statusParts[1])
	statusText := ""
	if len(statusParts) >= 3 {
		statusText = statusParts[2]
	}

	headers := make(map[string]string)
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		idx := strings.Index(line, ":")
		if idx != -1 {
			k := strings.ToLower(strings.TrimSpace(line[:idx]))
			v := strings.TrimSpace(line[idx+1:])
			headers[k] = v
		}
	}

	var respBody []byte
	if clStr, ok := headers["content-length"]; ok {
		cl, _ := strconv.Atoi(clStr)
		if cl > 0 {
			respBody = make([]byte, cl)
			if _, err := io.ReadFull(c.reader, respBody); err != nil {
				return nil, err
			}
		}
	}

	// Reset deadline
	_ = c.conn.SetDeadline(time.Time{})

	resp := &Response{
		StatusCode: statusCode,
		StatusText: statusText,
		Proto:      statusParts[0],
		Headers:    headers,
		Body:       respBody,
	}

	// Extract Session header
	if sHdr, ok := headers["session"]; ok {
		parts := strings.Split(sHdr, ";")
		c.sessionID = strings.TrimSpace(parts[0])
	}

	// Handle 401 challenge and retry once
	if statusCode == 401 && c.Username != "" && c.authChallenge == nil {
		authVal, ok := headers["www-authenticate"]
		if ok {
			ch, err := httpauth.ParseDigestChallenge(authVal)
			if err == nil {
				c.authChallenge = ch
				c.nonceCount = 0
				// Retry authenticated
				return c.sendRequest(method, reqURI, extraHeaders, body)
			}
		}
	}

	return resp, nil
}

// Options sends an RTSP OPTIONS request
func (c *Client) Options() ([]string, error) {
	resp, err := c.sendRequest("OPTIONS", c.TargetURL, nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("OPTIONS failed with status %d: %s", resp.StatusCode, resp.StatusText)
	}

	public := resp.Headers["public"]
	var methods []string
	for _, m := range strings.Split(public, ",") {
		m = strings.TrimSpace(m)
		if m != "" {
			methods = append(methods, m)
		}
	}
	return methods, nil
}

// Describe sends an RTSP DESCRIBE request and parses the SDP presentation
func (c *Client) Describe() (*SDPInfo, error) {
	headers := map[string]string{
		"Accept": "application/sdp",
	}
	resp, err := c.sendRequest("DESCRIBE", c.TargetURL, headers, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 401 {
		return nil, ErrRTSPUnauthorized
	}
	if resp.StatusCode == 404 {
		return nil, ErrRTSPNotFound
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("DESCRIBE failed with status %d: %s", resp.StatusCode, resp.StatusText)
	}

	sdp := ParseSDP(string(resp.Body))
	c.sdpInfo = sdp
	return sdp, nil
}

// Setup sends an RTSP SETUP request for a media track in TCP-interleaved mode
func (c *Client) Setup(trackControlURL string, channelIndex int) (string, error) {
	controlURI := trackControlURL
	if !strings.HasPrefix(controlURI, "rtsp://") {
		// Relative URL - append to base TargetURL
		if strings.HasSuffix(c.TargetURL, "/") || strings.HasPrefix(controlURI, "/") {
			controlURI = strings.TrimRight(c.TargetURL, "/") + "/" + strings.TrimLeft(controlURI, "/")
		} else {
			controlURI = c.TargetURL + "/" + controlURI
		}
	}

	rtpCh := channelIndex * 2
	rtcpCh := rtpCh + 1
	transportHdr := fmt.Sprintf("RTP/AVP/TCP;unicast;interleaved=%d-%d", rtpCh, rtcpCh)

	headers := map[string]string{
		"Transport": transportHdr,
	}

	resp, err := c.sendRequest("SETUP", controlURI, headers, nil)
	if err != nil {
		return "", err
	}

	if resp.StatusCode == 461 {
		return "", ErrRTSPTransportFail
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("SETUP failed with status %d: %s", resp.StatusCode, resp.StatusText)
	}

	return resp.Headers["transport"], nil
}

// Play sends an RTSP PLAY request to start video delivery
func (c *Client) Play() error {
	headers := map[string]string{
		"Range": "npt=0.000-",
	}
	resp, err := c.sendRequest("PLAY", c.TargetURL, headers, nil)
	if err != nil {
		return err
	}

	if resp.StatusCode == 454 {
		return ErrRTSPSessionInvalid
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("PLAY failed with status %d: %s", resp.StatusCode, resp.StatusText)
	}
	return nil
}

// ReadPacket reads a single interleaved RTP/RTCP packet from the connection
func (c *Client) ReadPacket() (*RTPPacket, error) {
	if !c.connected || c.conn == nil {
		return nil, ErrRTSPNotConnected
	}
	return ReadInterleavedPacket(c.reader)
}

// Teardown cleanly stops the stream session
func (c *Client) Teardown() error {
	if !c.connected || c.conn == nil {
		return nil
	}
	_, err := c.sendRequest("TEARDOWN", c.TargetURL, nil, nil)
	_ = c.Close()
	return err
}

// SessionID returns the current RTSP session token
func (c *Client) SessionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID
}
