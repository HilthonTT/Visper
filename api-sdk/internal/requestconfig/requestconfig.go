package requestconfig

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"time"

	"github.com/hilthontt/visper/api-sdk/internal"
)

// This interface is primarily used to describe an [*http.Client], but also
// supports custom HTTP implementations.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// RequestConfig represents all the state related to one request.
//
// Editing the variables inside RequestConfig directly is unstable api. Prefer
// composing the RequestOption instead if possible.
type RequestConfig struct {
	MaxRetries     int
	RequestTimeout time.Duration
	Context        context.Context
	Request        *http.Request
	BaseURL        *url.URL
	// DefaultBaseURL will be used if BaseURL is not explicitly overridden using
	// WithBaseURL.
	DefaultBaseURL *url.URL
	CustomHTTPDoer HTTPDoer
	HTTPClient     *http.Client
	Middlewares    []middleware
	BearerToken    string
	AppID          string
	// If ResponseBodyInto not nil, then we will attempt to deserialize into
	// ResponseBodyInto. If Destination is a []byte, then it will return the body as
	// is.
	ResponseBodyInto any
	// ResponseInto copies the \*http.Response of the corresponding request into the
	// given address
	ResponseInto **http.Response
	Body         io.Reader
}

// middleware is exactly the same type as the Middleware type found in the [option] package,
// but it is redeclared here for circular dependency issues.
type middleware = func(*http.Request, middlewareNext) (*http.Response, error)

// middlewareNext is exactly the same type as the MiddlewareNext type found in the [option] package,
// but it is redeclared here for circular dependency issues.
type middlewareNext = func(*http.Request) (*http.Response, error)

type RequestOption interface {
	Apply()
}

type RequestOptionFunc func(*RequestConfig) error
type PreRequestOptionFunc func(*RequestConfig) error

func (s RequestOptionFunc) Apply(r *RequestConfig) error {
	return s(r)
}

func (s PreRequestOptionFunc) Apply(r *RequestConfig) error {
	return s(r)
}

func getDefaultHeaders() map[string]string {
	return map[string]string{
		"User-Agent": fmt.Sprintf("Visper/Client %s", internal.PackageVersion),
	}
}

func getNormalizedOS() string {
	switch runtime.GOOS {
	case "ios":
		return "iOS"
	case "android":
		return "Android"
	case "darwin":
		return "MacOS"
	case "window":
		return "Windows"
	case "freebsd":
		return "FreeBSD"
	case "openbsd":
		return "OpenBSD"
	case "linux":
		return "Linux"
	default:
		return fmt.Sprintf("Other:%s", runtime.GOOS)
	}
}

func getNormalizedArchitecture() string {
	switch runtime.GOARCH {
	case "386":
		return "x32"
	case "amd64":
		return "x64"
	case "arm":
		return "arm"
	case "arm64":
		return "arm64"
	default:
		return fmt.Sprintf("other:%s", runtime.GOARCH)
	}
}

func getPlatformProperties() map[string]string {
	return map[string]string{
		"X-Stainless-Lang":            "go",
		"X-Stainless-Package-Version": internal.PackageVersion,
		"X-Stainless-OS":              getNormalizedOS(),
		"X-Stainless-Arch":            getNormalizedArchitecture(),
		"X-Stainless-Runtime":         "go",
		"X-Stainless-Runtime-Version": runtime.Version(),
	}
}
