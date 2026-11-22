package middleware

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"regexp"
)

const headerRequestID = "X-Request-Id"

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

type contextKey struct{}

// RequestID ensures every request has an X-Request-Id on the context and response.
// A missing or malformed inbound id is replaced so logs cannot be poisoned by
// control characters or oversized values.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerRequestID)
		if !requestIDPattern.MatchString(id) {
			id = newRequestID()
		}
		w.Header().Set(headerRequestID, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, id)))
	})
}

// RequestIDFromContext returns the request id, or an empty string when absent.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(contextKey{}).(string)
	return id
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("middleware: crypto/rand failed: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
