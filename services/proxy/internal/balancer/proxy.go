package balancer

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"
)

// statusClientClosedRequest is the de-facto (nginx) status for requests the
// client abandoned before the backend answered.
const statusClientClosedRequest = 499

// newProxyTransport creates the transport shared by all backends.
func newProxyTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100, // all traffic goes to a handful of hosts
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		// HTTP/1.1 only so Upgrade (WebSocket) requests can be proxied.
		ForceAttemptHTTP2: false,
	}
}

// newReverseProxy builds the reverse proxy for b. Rewrite (unlike Director)
// strips client-supplied Forwarded/X-Forwarded-* headers before they reach
// the backend, and ReverseProxy itself removes hop-by-hop headers while
// passing Connection: Upgrade through for WebSockets.
func (lb *LoadBalancer) newReverseProxy(b *Backend) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(b.URL) // also sets the outbound Host to the target host
			pr.SetXForwarded()
			if prior := pr.In.Header.Values("X-Forwarded-For"); len(prior) > 0 && lb.trustForwarded(pr.In) {
				pr.Out.Header.Set("X-Forwarded-For",
					strings.Join(prior, ", ")+", "+pr.Out.Header.Get("X-Forwarded-For"))
			}
			pr.Out.Header.Set("X-Real-IP", lb.clientIP(pr.In))
		},
		Transport:    lb.transport,
		ErrorHandler: lb.proxyErrorHandler(b),
	}
}

func (lb *LoadBalancer) proxyErrorHandler(b *Backend) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		if errors.Is(err, context.Canceled) && r.Context().Err() != nil {
			// The client went away; not the backend's fault.
			w.WriteHeader(statusClientClosedRequest)
			return
		}

		label := b.label()
		logf("proxy error for %s: %v", label, err)
		lb.metrics.BackendErrors.WithLabelValues(label, "proxy_error").Inc()

		lb.mu.Lock()
		maxFail := lb.maxFailCount
		lb.mu.Unlock()
		if b.recordFailure(maxFail) {
			lb.setBackendUp(b, false)
			logf("backend %s marked down after %d consecutive errors", label, maxFail)
		}

		status := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		http.Error(w, http.StatusText(status), status)
	}
}

// isWebSocketRequest reports whether r asks for a WebSocket upgrade.
func isWebSocketRequest(r *http.Request) bool {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return false
	}
	for _, v := range r.Header.Values("Connection") {
		for tok := range strings.SplitSeq(v, ",") {
			if strings.EqualFold(strings.TrimSpace(tok), "upgrade") {
				return true
			}
		}
	}
	return false
}

// statusRecorder captures the response status. It implements Unwrap so
// http.ResponseController (used by ReverseProxy for flushing and WebSocket
// hijacking) still reaches the underlying writer.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func newStatusRecorder(w http.ResponseWriter) *statusRecorder {
	return &statusRecorder{ResponseWriter: w, status: http.StatusOK}
}

func (w *statusRecorder) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		// 1xx informational responses (other than 101) may be followed by
		// the final status.
		w.wroteHeader = code >= 200 || code == http.StatusSwitchingProtocols
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusRecorder) Write(b []byte) (int, error) {
	w.wroteHeader = true
	return w.ResponseWriter.Write(b)
}

func (w *statusRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }
