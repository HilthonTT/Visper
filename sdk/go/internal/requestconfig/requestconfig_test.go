package requestconfig

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBodyWithTimeoutPropagatesEOF(t *testing.T) {
	stopped := false
	b := &bodyWithTimeout{rc: io.NopCloser(strings.NewReader("hello")), stop: func() { stopped = true }}

	got, err := io.ReadAll(b) // would loop forever if EOF were swallowed
	if err != nil || string(got) != "hello" {
		t.Fatalf("ReadAll = %q, %v", got, err)
	}
	if err := b.Close(); err != nil || !stopped {
		t.Fatalf("Close = %v, stopped = %v", err, stopped)
	}
}

func TestGetNormalizedOS(t *testing.T) {
	got := getNormalizedOS()
	if strings.HasPrefix(got, "Other:") {
		switch runtime.GOOS {
		case "windows", "linux", "darwin":
			t.Fatalf("getNormalizedOS() = %q for GOOS %q", got, runtime.GOOS)
		}
	}
}

func execute(t *testing.T, h http.HandlerFunc, dst any, opts ...RequestOption) error {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL + "/")
	opts = append([]RequestOption{RequestOptionFunc(func(rc *RequestConfig) error {
		rc.BaseURL = u
		return nil
	})}, opts...)
	return ExecuteNewRequest(context.Background(), http.MethodGet, "thing", nil, dst, opts...)
}

func TestExecuteRetriesServerErrors(t *testing.T) {
	var calls atomic.Int32
	var dst map[string]any
	err := execute(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 2 {
			w.Header().Set("Retry-After-Ms", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}, &dst)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || dst["ok"] != true {
		t.Fatalf("calls = %d, dst = %v", calls.Load(), dst)
	}
}

func TestExecuteNilDestinationClosesBody(t *testing.T) {
	err := execute(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ignored")
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestExecuteRequestTimeout(t *testing.T) {
	err := execute(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}, nil, RequestOptionFunc(func(rc *RequestConfig) error {
		rc.RequestTimeout = 50 * time.Millisecond
		rc.MaxRetries = 0
		return nil
	}))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
}

func TestExecuteNonJSONIntoString(t *testing.T) {
	var s string
	err := execute(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "plain")
	}, &s)
	if err != nil || s != "plain" {
		t.Fatalf("s = %q, err = %v", s, err)
	}
}
