package httpauth

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseDigestChallenge(t *testing.T) {
	header := `Digest realm="Hikvision", nonce="58494b564953494f4e", qop="auth", stale="false", opaque="12345"`
	challenge, err := ParseDigestChallenge(header)
	if err != nil {
		t.Fatalf("unexpected error parsing challenge: %v", err)
	}

	if challenge.Realm != "Hikvision" {
		t.Errorf("expected realm 'Hikvision', got '%s'", challenge.Realm)
	}
	if challenge.Nonce != "58494b564953494f4e" {
		t.Errorf("expected nonce '58494b564953494f4e', got '%s'", challenge.Nonce)
	}
	if challenge.Qop != "auth" {
		t.Errorf("expected qop 'auth', got '%s'", challenge.Qop)
	}
	if challenge.Opaque != "12345" {
		t.Errorf("expected opaque '12345', got '%s'", challenge.Opaque)
	}
	if challenge.Stale {
		t.Errorf("expected stale false")
	}
	if challenge.Algorithm != "MD5" {
		t.Errorf("expected algorithm MD5, got '%s'", challenge.Algorithm)
	}
}

func TestComputeDigestResponse(t *testing.T) {
	// RFC 2617 test vector check
	challenge := &Challenge{
		Realm:     "testrealm@host.com",
		Nonce:     "dcd98b7102dd2f0e8b11d0f600bfb0c093",
		Qop:       "auth",
		Opaque:    "5ccc069c403ebaf9f0171e9517f40e41",
		Algorithm: "MD5",
	}

	username := "Mufasa"
	password := "Circle Of Life"
	method := "GET"
	uri := "/dir/index.html"
	nc := uint32(1)
	cnonce := "0a4f113b"

	authHeader, err := ComputeDigestResponse(challenge, username, password, method, uri, nc, cnonce)
	if err != nil {
		t.Fatalf("ComputeDigestResponse failed: %v", err)
	}

	// RFC 2617 expected response is 6629fae49393a05397450978507c4ef1
	expectedResponse := "6629fae49393a05397450978507c4ef1"
	if !strings.Contains(authHeader, fmt.Sprintf(`response="%s"`, expectedResponse)) {
		t.Errorf("expected response '%s' in header, got: %s", expectedResponse, authHeader)
	}
}

func TestDigestTransportRoundTrip(t *testing.T) {
	validUser := "admin"
	validPass := "Secret123!"
	realm := "CCTV-Security"
	serverNonce := "abcdef123456"

	// Mock server that returns 401 on unauthenticated requests and 200 on valid digest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Digest realm="%s", nonce="%s", qop="auth"`, realm, serverNonce))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if !strings.HasPrefix(auth, "Digest ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Simple validation that the correct credentials were sent
		if strings.Contains(auth, fmt.Sprintf(`username="%s"`, validUser)) &&
			strings.Contains(auth, fmt.Sprintf(`realm="%s"`, realm)) &&
			strings.Contains(auth, fmt.Sprintf(`nonce="%s"`, serverNonce)) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `<Response>Authenticated OK</Response>`)
			return
		}

		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := NewClient(validUser, validPass, 5*time.Second, false)

	req, err := http.NewRequest("GET", server.URL+"/api/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK after digest challenge, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Authenticated OK") {
		t.Fatalf("unexpected body: %s", string(body))
	}
}
