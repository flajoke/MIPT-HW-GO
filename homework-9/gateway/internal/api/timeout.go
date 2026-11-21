package api

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"time"
)

const DefaultRequestTimeout = 2 * time.Second

// Cancellation is cooperative: services and SQL operations must honor ctx.
// Buffering prevents a late successful/partial response from replacing a 504.
// No detached HTTP-handler goroutine is created.
func TimeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if err := ctx.Err(); err != nil {
				writeError(w, http.StatusGatewayTimeout, "request timeout")
				return
			}
			controller := http.NewResponseController(w)
			// Prevent an incomplete request body from blocking JSON decoding indefinitely.
			if deadline, ok := ctx.Deadline(); ok {
				if err := controller.SetReadDeadline(deadline); err == nil {
					defer controller.SetReadDeadline(time.Time{})
				}
			}
			buffered := &responseBuffer{header: make(http.Header)}
			next.ServeHTTP(buffered, r.WithContext(ctx))
			if err := ctx.Err(); err != nil {
				log.Printf("request canceled: %s %s: %v", r.Method, r.URL.Path, err)
				writeError(w, http.StatusGatewayTimeout, "request timeout")
				return
			}
			for key, values := range buffered.header {
				w.Header()[key] = append([]string(nil), values...)
			}
			code := buffered.status
			if code == 0 {
				code = http.StatusOK
			}
			w.WriteHeader(code)
			if _, err := w.Write(buffered.body.Bytes()); err != nil {
				log.Printf("write response: %v", err)
			}
		})
	}
}

type responseBuffer struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *responseBuffer) Header() http.Header { return w.header }
func (w *responseBuffer) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}
func (w *responseBuffer) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}
