package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCookieSecurity_DirectHTTPS(t *testing.T) {
	cfg := CookieConfig{
		SecureCookies:     false,
		UseHostPrefix:     true,
		SessionCookieName: "sentinel_session",
		CSRFCookieName:    "sentinel_csrf",
		TrustedProxies:    []string{"127.0.0.1"},
		SameSite:          http.SameSiteStrictMode,
	}
	s := &Server{cookieConfig: cfg}

	req := httptest.NewRequest("GET", "https://example.com/api/v1/auth", nil)
	req.TLS = &tls.ConnectionState{} // Simulate direct TLS termination
	w := httptest.NewRecorder()

	s.setSessionCookie(w, req, "test-session-tok-123", time.Now().Add(1*time.Hour))
	s.setCSRFCookie(w, req, "test-csrf-tok-456", time.Now().Add(1*time.Hour))

	cookies := w.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("Expected 2 cookies, got %d", len(cookies))
	}

	var sessCookie, csrfCookie *http.Cookie
	for _, c := range cookies {
		if strings.Contains(c.Name, "sentinel_session") {
			sessCookie = c
		}
		if strings.Contains(c.Name, "sentinel_csrf") {
			csrfCookie = c
		}
	}

	if sessCookie == nil {
		t.Fatalf("Session cookie not found")
	}
	if sessCookie.Name != "__Host-sentinel_session" {
		t.Errorf("Expected cookie name __Host-sentinel_session, got %s", sessCookie.Name)
	}
	if !sessCookie.Secure {
		t.Errorf("Session cookie must be Secure under HTTPS")
	}
	if !sessCookie.HttpOnly {
		t.Errorf("Session cookie must be HttpOnly")
	}
	if sessCookie.Path != "/" {
		t.Errorf("Session cookie Path must be '/', got %s", sessCookie.Path)
	}
	if sessCookie.Domain != "" {
		t.Errorf("Session cookie Domain must be empty for __Host- compliance, got %s", sessCookie.Domain)
	}
	if sessCookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("Session cookie SameSite must be Strict, got %v", sessCookie.SameSite)
	}

	if csrfCookie == nil {
		t.Fatalf("CSRF cookie not found")
	}
	if csrfCookie.Name != "__Host-sentinel_csrf" {
		t.Errorf("Expected CSRF cookie name __Host-sentinel_csrf, got %s", csrfCookie.Name)
	}
	if !csrfCookie.Secure {
		t.Errorf("CSRF cookie must be Secure under HTTPS")
	}
	if csrfCookie.HttpOnly {
		t.Errorf("CSRF cookie must NOT be HttpOnly so frontend JS can read it")
	}
	if csrfCookie.Path != "/" {
		t.Errorf("CSRF cookie Path must be '/', got %s", csrfCookie.Path)
	}
	if csrfCookie.Domain != "" {
		t.Errorf("CSRF cookie Domain must be empty, got %s", csrfCookie.Domain)
	}
}

func TestCookieSecurity_DevelopmentHTTP(t *testing.T) {
	cfg := CookieConfig{
		SecureCookies:     false,
		UseHostPrefix:     true, // Even if true, insecure HTTP cannot use __Host-
		SessionCookieName: "sentinel_session",
		CSRFCookieName:    "sentinel_csrf",
		TrustedProxies:    []string{"127.0.0.1"},
		SameSite:          http.SameSiteStrictMode,
	}
	s := &Server{cookieConfig: cfg}

	req := httptest.NewRequest("GET", "http://localhost:8080/api/v1/auth", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	w := httptest.NewRecorder()

	s.setSessionCookie(w, req, "dev-session-tok", time.Now().Add(1*time.Hour))
	s.setCSRFCookie(w, req, "dev-csrf-tok", time.Now().Add(1*time.Hour))

	cookies := w.Result().Cookies()
	var sessCookie, csrfCookie *http.Cookie
	for _, c := range cookies {
		if strings.Contains(c.Name, "sentinel_session") {
			sessCookie = c
		}
		if strings.Contains(c.Name, "sentinel_csrf") {
			csrfCookie = c
		}
	}

	if sessCookie == nil {
		t.Fatalf("Session cookie not found")
	}
	if sessCookie.Name != "sentinel_session" {
		t.Errorf("Insecure HTTP must NOT use __Host- prefix, got name %s", sessCookie.Name)
	}
	if sessCookie.Secure {
		t.Errorf("Session cookie must not be Secure on development HTTP")
	}
	if !sessCookie.HttpOnly {
		t.Errorf("Session cookie must remain HttpOnly on development HTTP")
	}
	if sessCookie.Path != "/" {
		t.Errorf("Session cookie Path must be '/', got %s", sessCookie.Path)
	}

	if csrfCookie == nil {
		t.Fatalf("CSRF cookie not found")
	}
	if csrfCookie.Name != "sentinel_csrf" {
		t.Errorf("Insecure HTTP CSRF must NOT use __Host- prefix, got %s", csrfCookie.Name)
	}
	if csrfCookie.Secure {
		t.Errorf("CSRF cookie must not be Secure on development HTTP")
	}
	if csrfCookie.HttpOnly {
		t.Errorf("CSRF cookie must NOT be HttpOnly")
	}
}

func TestCookieSecurity_TrustedReverseProxy(t *testing.T) {
	cfg := CookieConfig{
		SecureCookies:     false,
		UseHostPrefix:     true,
		SessionCookieName: "sentinel_session",
		CSRFCookieName:    "sentinel_csrf",
		TrustedProxies:    []string{"127.0.0.1", "10.0.0.0/8"},
		SameSite:          http.SameSiteStrictMode,
	}
	s := &Server{cookieConfig: cfg}

	// 1. Trusted proxy with X-Forwarded-Proto: https
	reqTrusted := httptest.NewRequest("GET", "http://10.0.1.50:8080/api/v1/auth", nil)
	reqTrusted.RemoteAddr = "10.0.1.50:49152"
	reqTrusted.Header.Set("X-Forwarded-Proto", "https")
	wTrusted := httptest.NewRecorder()

	s.setSessionCookie(wTrusted, reqTrusted, "trusted-sess", time.Now().Add(1*time.Hour))
	cookiesTrusted := wTrusted.Result().Cookies()
	if len(cookiesTrusted) == 0 {
		t.Fatalf("No cookies received")
	}
	if !cookiesTrusted[0].Secure {
		t.Errorf("Trusted proxy with X-Forwarded-Proto: https must yield Secure: true")
	}
	if cookiesTrusted[0].Name != "__Host-sentinel_session" {
		t.Errorf("Expected __Host-sentinel_session, got %s", cookiesTrusted[0].Name)
	}

	// 2. Untrusted client attempting to spoof X-Forwarded-Proto: https
	reqSpoofed := httptest.NewRequest("GET", "http://192.168.1.100:8080/api/v1/auth", nil)
	reqSpoofed.RemoteAddr = "192.168.1.100:51234"
	reqSpoofed.Header.Set("X-Forwarded-Proto", "https")
	wSpoofed := httptest.NewRecorder()

	s.setSessionCookie(wSpoofed, reqSpoofed, "spoofed-sess", time.Now().Add(1*time.Hour))
	cookiesSpoofed := wSpoofed.Result().Cookies()
	if len(cookiesSpoofed) == 0 {
		t.Fatalf("No cookies received")
	}
	if cookiesSpoofed[0].Secure {
		t.Errorf("Untrusted client sending X-Forwarded-Proto: https must be IGNORED, but cookie was Secure")
	}
	if cookiesSpoofed[0].Name != "sentinel_session" {
		t.Errorf("Untrusted client on HTTP must not get __Host- prefix, got %s", cookiesSpoofed[0].Name)
	}
}

func TestCookieSecurity_ExplicitSecureCookiesOverride(t *testing.T) {
	cfg := CookieConfig{
		SecureCookies:     true, // Explicitly enforce secure cookies in production
		UseHostPrefix:     true,
		SessionCookieName: "sentinel_session",
		CSRFCookieName:    "sentinel_csrf",
		TrustedProxies:    nil,
		SameSite:          http.SameSiteStrictMode,
	}
	s := &Server{cookieConfig: cfg}

	// Plain HTTP request without TLS or proxy headers
	req := httptest.NewRequest("GET", "http://prod.internal:8080/api/v1/auth", nil)
	req.RemoteAddr = "172.16.0.5:12345"
	w := httptest.NewRecorder()

	s.setSessionCookie(w, req, "override-sess", time.Now().Add(1*time.Hour))
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("No cookies received")
	}
	if !cookies[0].Secure {
		t.Errorf("Explicit SecureCookies=true must always produce Secure: true")
	}
	if cookies[0].Name != "__Host-sentinel_session" {
		t.Errorf("Explicit SecureCookies=true with UseHostPrefix=true must produce __Host- prefix")
	}
}

func TestCookieSecurity_ExtractionHelpers(t *testing.T) {
	cfg := CookieConfig{
		SessionCookieName: "sentinel_session",
		CSRFCookieName:    "sentinel_csrf",
	}
	s := &Server{cookieConfig: cfg}

	// Request with standard cookies
	reqStd := httptest.NewRequest("GET", "http://localhost/", nil)
	reqStd.AddCookie(&http.Cookie{Name: "sentinel_session", Value: "sess-standard"})
	reqStd.AddCookie(&http.Cookie{Name: "sentinel_csrf", Value: "csrf-standard"})

	if tok, isCookie := s.extractSessionToken(reqStd); tok != "sess-standard" || !isCookie {
		t.Errorf("Failed to extract standard session token: got %s, isCookie: %v", tok, isCookie)
	}
	if tok := s.extractCSRFTokenFromCookie(reqStd); tok != "csrf-standard" {
		t.Errorf("Failed to extract standard CSRF token: got %s", tok)
	}

	// Request with __Host- prefixed cookies
	reqHost := httptest.NewRequest("GET", "https://localhost/", nil)
	reqHost.AddCookie(&http.Cookie{Name: "__Host-sentinel_session", Value: "sess-host"})
	reqHost.AddCookie(&http.Cookie{Name: "__Host-sentinel_csrf", Value: "csrf-host"})

	if tok, isCookie := s.extractSessionToken(reqHost); tok != "sess-host" || !isCookie {
		t.Errorf("Failed to extract __Host- session token: got %s, isCookie: %v", tok, isCookie)
	}
	if tok := s.extractCSRFTokenFromCookie(reqHost); tok != "csrf-host" {
		t.Errorf("Failed to extract __Host- CSRF token: got %s", tok)
	}
}
