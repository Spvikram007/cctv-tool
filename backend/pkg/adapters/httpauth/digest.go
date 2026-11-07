package httpauth

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrNoAuthChallenge = errors.New("no digest authentication challenge found in response")
	ErrUnsupportedAlgo = errors.New("unsupported digest authentication algorithm")
)

// Challenge represents parsed fields from a WWW-Authenticate: Digest header
type Challenge struct {
	Realm     string
	Nonce     string
	Qop       string
	Opaque    string
	Algorithm string
	Stale     bool
}

// ParseDigestChallenge parses a WWW-Authenticate header value into a Challenge struct
func ParseDigestChallenge(header string) (*Challenge, error) {
	if !strings.HasPrefix(strings.ToLower(header), "digest ") {
		return nil, ErrNoAuthChallenge
	}

	params := strings.TrimPrefix(header, "Digest ")
	params = strings.TrimPrefix(params, "digest ")

	c := &Challenge{
		Algorithm: "MD5", // Default per RFC 2617
	}

	for _, part := range splitParams(params) {
		part = strings.TrimSpace(part)
		if idx := strings.Index(part, "="); idx != -1 {
			key := strings.ToLower(strings.TrimSpace(part[:idx]))
			val := strings.Trim(strings.TrimSpace(part[idx+1:]), `"`)

			switch key {
			case "realm":
				c.Realm = val
			case "nonce":
				c.Nonce = val
			case "qop":
				c.Qop = val
			case "opaque":
				c.Opaque = val
			case "algorithm":
				c.Algorithm = strings.ToUpper(val)
			case "stale":
				c.Stale = strings.ToLower(val) == "true"
			}
		}
	}

	if c.Nonce == "" {
		return nil, errors.New("digest challenge missing nonce")
	}

	return c, nil
}

func splitParams(s string) []string {
	var parts []string
	var current strings.Builder
	inQuotes := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '"' {
			inQuotes = !inQuotes
			current.WriteByte(ch)
		} else if ch == ',' && !inQuotes {
			parts = append(parts, current.String())
			current.Reset()
		} else {
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// ComputeDigestResponse computes the RFC 2617 / RFC 7616 Authorization header
func ComputeDigestResponse(c *Challenge, username, password, method, uriStr string, nc uint32, cnonce string) (string, error) {
	// Parse URI path and query for digest uri parameter
	u, err := url.Parse(uriStr)
	digestURI := uriStr
	if err == nil && u.RequestURI() != "" {
		digestURI = u.RequestURI()
	}

	ncStr := fmt.Sprintf("%08x", nc)

	var ha1 string
	switch c.Algorithm {
	case "MD5", "":
		h := md5.New()
		h.Write([]byte(fmt.Sprintf("%s:%s:%s", username, c.Realm, password)))
		ha1 = hex.EncodeToString(h.Sum(nil))
	case "MD5-SESS":
		h := md5.New()
		h.Write([]byte(fmt.Sprintf("%s:%s:%s", username, c.Realm, password)))
		ha1Pre := hex.EncodeToString(h.Sum(nil))
		h.Reset()
		h.Write([]byte(fmt.Sprintf("%s:%s:%s", ha1Pre, c.Nonce, cnonce)))
		ha1 = hex.EncodeToString(h.Sum(nil))
	case "SHA-256":
		h := sha256.New()
		h.Write([]byte(fmt.Sprintf("%s:%s:%s", username, c.Realm, password)))
		ha1 = hex.EncodeToString(h.Sum(nil))
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedAlgo, c.Algorithm)
	}

	var ha2 string
	if c.Algorithm == "SHA-256" {
		h := sha256.New()
		h.Write([]byte(fmt.Sprintf("%s:%s", method, digestURI)))
		ha2 = hex.EncodeToString(h.Sum(nil))
	} else {
		h := md5.New()
		h.Write([]byte(fmt.Sprintf("%s:%s", method, digestURI)))
		ha2 = hex.EncodeToString(h.Sum(nil))
	}

	var response string
	qopChosen := ""
	if strings.Contains(strings.ToLower(c.Qop), "auth") {
		qopChosen = "auth"
		h := md5.New()
		if c.Algorithm == "SHA-256" {
			h256 := sha256.New()
			h256.Write([]byte(fmt.Sprintf("%s:%s:%s:%s:%s:%s", ha1, c.Nonce, ncStr, cnonce, qopChosen, ha2)))
			response = hex.EncodeToString(h256.Sum(nil))
		} else {
			h.Write([]byte(fmt.Sprintf("%s:%s:%s:%s:%s:%s", ha1, c.Nonce, ncStr, cnonce, qopChosen, ha2)))
			response = hex.EncodeToString(h.Sum(nil))
		}
	} else {
		// No qop
		h := md5.New()
		h.Write([]byte(fmt.Sprintf("%s:%s:%s", ha1, c.Nonce, ha2)))
		response = hex.EncodeToString(h.Sum(nil))
	}

	var authHeader strings.Builder
	authHeader.WriteString(fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`,
		username, c.Realm, c.Nonce, digestURI, response))

	if qopChosen != "" {
		authHeader.WriteString(fmt.Sprintf(`, qop=%s, nc=%s, cnonce="%s"`, qopChosen, ncStr, cnonce))
	}
	if c.Opaque != "" {
		authHeader.WriteString(fmt.Sprintf(`, opaque="%s"`, c.Opaque))
	}
	if c.Algorithm != "" && c.Algorithm != "MD5" {
		authHeader.WriteString(fmt.Sprintf(`, algorithm=%s`, c.Algorithm))
	}

	return authHeader.String(), nil
}

// GenerateCNonce generates a cryptographically random client nonce
func GenerateCNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to time-based hex if rand fails
		return fmt.Sprintf("%016x", atomic.AddUint64(&globalNonceCounter, 1))
	}
	return hex.EncodeToString(b)
}

var globalNonceCounter uint64

// DigestTransport handles automatic challenge-response Digest and Basic HTTP authentication
type DigestTransport struct {
	Transport     http.RoundTripper
	Username      string
	Password      string
	mu            sync.Mutex
	lastChallenge *Challenge
	nonceCount    uint32
}

func NewDigestTransport(username, password string, baseRoundTripper http.RoundTripper) *DigestTransport {
	if baseRoundTripper == nil {
		baseRoundTripper = http.DefaultTransport
	}
	return &DigestTransport{
		Transport: baseRoundTripper,
		Username:  username,
		Password:  password,
	}
}

func (t *DigestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// If credentials are empty, pass through directly
	if t.Username == "" {
		return t.Transport.RoundTrip(req)
	}

	t.mu.Lock()
	cachedChallenge := t.lastChallenge
	t.mu.Unlock()

	// If we already have a challenge from a previous 401, preemptively send Digest auth
	if cachedChallenge != nil {
		reqClone := req.Clone(req.Context())
		nc := atomic.AddUint32(&t.nonceCount, 1)
		cnonce := GenerateCNonce()
		authHeader, err := ComputeDigestResponse(cachedChallenge, t.Username, t.Password, reqClone.Method, reqClone.URL.String(), nc, cnonce)
		if err == nil {
			reqClone.Header.Set("Authorization", authHeader)
			resp, err := t.Transport.RoundTrip(reqClone)
			if err == nil && resp.StatusCode != http.StatusUnauthorized {
				return resp, nil
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}

	// First attempt without or with initial Basic header
	firstReq := req.Clone(req.Context())
	// Rewind body if present
	if req.GetBody != nil {
		var err error
		firstReq.Body, err = req.GetBody()
		if err != nil {
			return nil, err
		}
	}

	resp, err := t.Transport.RoundTrip(firstReq)
	if err != nil {
		return nil, err
	}

	// If not 401 Unauthorized, return response
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	authHeader := resp.Header.Get("WWW-Authenticate")
	if authHeader == "" {
		return resp, nil
	}

	// Check if Digest challenge
	challenge, err := ParseDigestChallenge(authHeader)
	if err != nil {
		// Not digest or malformed - could be Basic
		if strings.HasPrefix(strings.ToLower(authHeader), "basic") {
			resp.Body.Close()
			basicReq := req.Clone(req.Context())
			if req.GetBody != nil {
				basicReq.Body, _ = req.GetBody()
			}
			basicReq.SetBasicAuth(t.Username, t.Password)
			return t.Transport.RoundTrip(basicReq)
		}
		return resp, nil
	}

	resp.Body.Close()

	// Update cached challenge
	t.mu.Lock()
	t.lastChallenge = challenge
	t.nonceCount = 0
	nc := atomic.AddUint32(&t.nonceCount, 1)
	t.mu.Unlock()

	cnonce := GenerateCNonce()
	digestHeader, err := ComputeDigestResponse(challenge, t.Username, t.Password, req.Method, req.URL.String(), nc, cnonce)
	if err != nil {
		return nil, err
	}

	retryReq := req.Clone(req.Context())
	if req.GetBody != nil {
		retryReq.Body, _ = req.GetBody()
	} else if req.Body != nil {
		// Drain and reset not possible without GetBody
		return nil, errors.New("cannot retry request with non-replayable body")
	}
	retryReq.Header.Set("Authorization", digestHeader)

	return t.Transport.RoundTrip(retryReq)
}

// NewClient creates an http.Client configured with DigestTransport and TLS skip verification option
func NewClient(username, password string, timeout time.Duration, skipTLS bool) *http.Client {
	baseTransport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}
	if skipTLS {
		baseTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &http.Client{
		Transport: NewDigestTransport(username, password, baseTransport),
		Timeout:   timeout,
	}
}
