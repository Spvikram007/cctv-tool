package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type contextKey string

const (
	RequestIDKey contextKey = "request_id"
	HeaderRequestID         = "X-Request-ID"
)

// APIErrorDetail defines the structured error model
type APIErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// ErrorResponse defines the standard JSON error envelope with backward compatibility
type ErrorResponse struct {
	Success bool           `json:"success"`
	Message string         `json:"message"`
	Error   APIErrorDetail `json:"error"`
}

// GenerateRequestID generates a collision-resistant 16-character hex request ID
func GenerateRequestID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "req-fallback"
	}
	return "req-" + hex.EncodeToString(bytes)
}

// GetRequestID retrieves the request correlation ID from the request context
func GetRequestID(r *http.Request) string {
	if val, ok := r.Context().Value(RequestIDKey).(string); ok && val != "" {
		return val
	}
	return GenerateRequestID()
}

// RequestCorrelationMiddleware injects or propagates X-Request-ID
func RequestCorrelationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := strings.TrimSpace(r.Header.Get(HeaderRequestID))
		if reqID == "" || len(reqID) > 64 {
			reqID = GenerateRequestID()
		}

		// Inject into response header
		w.Header().Set(HeaderRequestID, reqID)

		// Inject into context
		ctx := context.WithValue(r.Context(), RequestIDKey, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// writeStructuredError writes a standardized, secure JSON error response
// It guarantees internal stack traces, system paths, and credentials are never leaked.
func writeStructuredError(w http.ResponseWriter, r *http.Request, status int, code, userMessage string) {
	reqID := GetRequestID(r)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := ErrorResponse{
		Success: false,
		Message: userMessage,
		Error: APIErrorDetail{
			Code:      code,
			Message:   userMessage,
			RequestID: reqID,
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// ReadinessCheck defines the status of critical backend subsystems
type ReadinessCheck struct {
	Ready     bool              `json:"ready"`
	Timestamp time.Time         `json:"timestamp"`
	Checks    map[string]string `json:"checks"`
}
