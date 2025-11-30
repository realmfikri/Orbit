package server

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type contextKey string

const correlationIDKey contextKey = "correlation_id"

func (s *Server) wrap(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		correlationID := s.extractOrCreateCorrelationID(r)

		ctx := context.WithValue(r.Context(), correlationIDKey, correlationID)
		r = r.WithContext(ctx)

		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		recorder.Header().Set(s.correlationHeader, correlationID)

		handler(recorder, r)

		duration := time.Since(start)
		s.logger.Info("request completed",
			"path", r.URL.Path,
			"method", r.Method,
			"status", recorder.status,
			"duration_ms", duration.Milliseconds(),
			"correlation_id", correlationID,
		)

		apiLatency.WithLabelValues(r.Method, r.URL.Path, strconv.Itoa(recorder.status)).Observe(duration.Seconds())
	}
}

func (s *Server) extractOrCreateCorrelationID(r *http.Request) string {
	if existing := r.Header.Get(s.correlationHeader); existing != "" {
		return existing
	}
	return uuid.NewString()
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := r.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("hijacker not supported")
}

func correlationIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(correlationIDKey).(string); ok {
		return v
	}
	return ""
}
