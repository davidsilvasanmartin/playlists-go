package api

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

// statusRecorder wraps http.ResponseWriter so we can capture the HTTP status
// code that the handler writes. The standard ResponseWriter does not expose
// the status after the fact, so we intercept WriteHeader.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader intercepts the status code before delegating to the real writer.
// If the handler never calls WriteHeader explicitly (which happens when it only
// calls Write), the status defaults to 200, matching net/http behaviour.
func (sr *statusRecorder) WriteHeader(status int) {
	sr.status = status
	sr.ResponseWriter.WriteHeader(status)
}

// LoggingMiddleware returns middleware that logs one structured line per request.
func LoggingMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap the ResponseWriter so we can read the status after the
			// handler returns.  Default to 200 so routes that never call
			// WriteHeader explicitly are logged correctly.
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			// Call the real handler
			next.ServeHTTP(rec, r)

			// Log after the handler returns so we have the final status and the
			// accurate elapsed time
			logger.Info("request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.String("query", r.URL.RawQuery),
				zap.Int("status", rec.status),
				zap.Duration("duration", time.Since(start)),
				zap.String("remoteAddr", r.RemoteAddr),
			)
		})
	}
}
