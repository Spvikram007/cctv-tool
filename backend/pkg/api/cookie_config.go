package api

import (
	"net"
	"net/http"
	"strings"
	"time"
)

// CookieConfig defines production cookie security and reverse-proxy trust policy
type CookieConfig struct {
	// SecureCookies enforces the Secure flag on all cookies (production HTTPS mode)
	SecureCookies bool `json:"secure_cookies"`
	// UseHostPrefix enables the __Host- prefix on cookies in production HTTPS mode
	UseHostPrefix bool `json:"use_host_prefix"`
	// SessionCookieName overrides the session cookie name (default: "sentinel_session")
	SessionCookieName string `json:"session_cookie_name"`
	// CSRFCookieName overrides the CSRF cookie name (default: "sentinel_csrf")
	CSRFCookieName string `json:"csrf_cookie_name"`
	// TrustedProxies contains the list of trusted IP addresses or CIDRs for reverse proxy deployments
	TrustedProxies []string `json:"trusted_proxies"`
	// SameSite defines the SameSite mode for cookies (default: http.SameSiteStrictMode)
	SameSite http.SameSite `json:"same_site"`
}

// DefaultCookieConfig provides safe development defaults
var DefaultCookieConfig = CookieConfig{
	SecureCookies:     false, // development HTTP mode by default
	UseHostPrefix:     false,
	SessionCookieName: "sentinel_session",
	CSRFCookieName:    "sentinel_csrf",
	SameSite:          http.SameSiteStrictMode,
	TrustedProxies: []string{
		"127.0.0.1",
		"::1",
	},
}

// isRequestSecure determines if the request should be treated as HTTPS
// It never relies solely on r.TLS != nil, respecting explicit configuration and reverse-proxy trust
func (s *Server) isRequestSecure(r *http.Request) bool {
	if s.cookieConfig.SecureCookies {
		return true
	}
	if r.TLS != nil {
		return true
	}
	// Reverse-proxy forwarded protocol check:
	// NEVER trust arbitrary X-Forwarded-Proto unless the immediate client IP is an explicitly trusted proxy
	if s.isRemoteAddrTrustedProxy(r.RemoteAddr) {
		fwdProto := r.Header.Get("X-Forwarded-Proto")
		if strings.EqualFold(fwdProto, "https") {
			return true
		}
	}
	return false
}

// isRemoteAddrTrustedProxy verifies whether remoteAddr matches configured trusted reverse proxies
func (s *Server) isRemoteAddrTrustedProxy(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	host = strings.TrimSpace(host)
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	for _, entry := range s.cookieConfig.TrustedProxies {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			_, subnet, err := net.ParseCIDR(entry)
			if err == nil && subnet.Contains(ip) {
				return true
			}
		} else {
			trustedIP := net.ParseIP(entry)
			if trustedIP != nil && trustedIP.Equal(ip) {
				return true
			}
		}
	}
	return false
}

// getSessionCookieName returns the session cookie name depending on security mode and prefix settings
func (s *Server) getSessionCookieName(isSecure bool) string {
	if isSecure && s.cookieConfig.UseHostPrefix {
		return "__Host-sentinel_session"
	}
	if s.cookieConfig.SessionCookieName != "" {
		return s.cookieConfig.SessionCookieName
	}
	return "sentinel_session"
}

// getCSRFCookieName returns the CSRF cookie name depending on security mode and prefix settings
func (s *Server) getCSRFCookieName(isSecure bool) string {
	if isSecure && s.cookieConfig.UseHostPrefix {
		return "__Host-sentinel_csrf"
	}
	if s.cookieConfig.CSRFCookieName != "" {
		return s.cookieConfig.CSRFCookieName
	}
	return "sentinel_csrf"
}

// extractSessionToken extracts the bearer token from Authorization header or session cookies
func (s *Server) extractSessionToken(r *http.Request) (string, bool) {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer "), false
	} else if strings.HasPrefix(authHeader, "bearer ") {
		return strings.TrimPrefix(authHeader, "bearer "), false
	}

	// Try __Host- prefix first, then standard name, then custom name
	cookieNames := []string{
		"__Host-sentinel_session",
		"sentinel_session",
	}
	if s.cookieConfig.SessionCookieName != "" && s.cookieConfig.SessionCookieName != "sentinel_session" {
		cookieNames = append([]string{s.cookieConfig.SessionCookieName}, cookieNames...)
	}

	for _, name := range cookieNames {
		if cookie, err := r.Cookie(name); err == nil && cookie.Value != "" {
			return cookie.Value, true
		}
	}

	return "", false
}

// extractCSRFTokenFromCookie extracts the CSRF token from the browser cookie
func (s *Server) extractCSRFTokenFromCookie(r *http.Request) string {
	cookieNames := []string{
		"__Host-sentinel_csrf",
		"sentinel_csrf",
	}
	if s.cookieConfig.CSRFCookieName != "" && s.cookieConfig.CSRFCookieName != "sentinel_csrf" {
		cookieNames = append([]string{s.cookieConfig.CSRFCookieName}, cookieNames...)
	}

	for _, name := range cookieNames {
		if cookie, err := r.Cookie(name); err == nil && cookie.Value != "" {
			return cookie.Value
		}
	}
	return ""
}

// setSessionCookie creates and sets the session cookie with production security attributes
func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	isSecure := s.isRequestSecure(r)
	cookieName := s.getSessionCookieName(isSecure)
	sameSite := s.cookieConfig.SameSite
	if sameSite == 0 {
		sameSite = http.SameSiteStrictMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: sameSite,
		Secure:   isSecure,
	})
}

// setCSRFCookie creates and sets the CSRF cookie (accessible to frontend JS for header population)
func (s *Server) setCSRFCookie(w http.ResponseWriter, r *http.Request, csrfToken string, expiresAt time.Time) {
	isSecure := s.isRequestSecure(r)
	cookieName := s.getCSRFCookieName(isSecure)
	sameSite := s.cookieConfig.SameSite
	if sameSite == 0 {
		sameSite = http.SameSiteStrictMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    csrfToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: false,
		SameSite: sameSite,
		Secure:   isSecure,
	})
}

// clearAuthCookies revokes and expires all session and CSRF cookies across all naming variants
func (s *Server) clearAuthCookies(w http.ResponseWriter, r *http.Request) {
	isSecure := s.isRequestSecure(r)
	sameSite := s.cookieConfig.SameSite
	if sameSite == 0 {
		sameSite = http.SameSiteStrictMode
	}

	names := []string{
		"sentinel_session",
		"__Host-sentinel_session",
		"sentinel_csrf",
		"__Host-sentinel_csrf",
	}
	if s.cookieConfig.SessionCookieName != "" {
		names = append(names, s.cookieConfig.SessionCookieName)
	}
	if s.cookieConfig.CSRFCookieName != "" {
		names = append(names, s.cookieConfig.CSRFCookieName)
	}

	for _, name := range names {
		isHttpOnly := strings.Contains(name, "session")
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
			HttpOnly: isHttpOnly,
			SameSite: sameSite,
			Secure:   isSecure,
		})
	}
}

// SetCookieConfig configures production cookie settings and reverse-proxy trust
func (s *Server) SetCookieConfig(cfg CookieConfig) {
	if cfg.SameSite == 0 {
		cfg.SameSite = http.SameSiteStrictMode
	}
	s.cookieConfig = cfg
}

// GetCookieConfig retrieves the current cookie security configuration
func (s *Server) GetCookieConfig() CookieConfig {
	return s.cookieConfig
}

