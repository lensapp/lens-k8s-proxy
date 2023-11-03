package proxy

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/httpstream"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/proxy"
	proxyutil "k8s.io/apimachinery/pkg/util/proxy"
	"k8s.io/apiserver/pkg/authentication/user"
	registryrest "k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/transport"
	"k8s.io/klog/v2"
)

type SecretGetterFunc func(context.Context, string, string) (*corev1.Secret, error)

// Server is a http.Handler which proxies Kubernetes APIs to remote API server.
type Server struct {
	handler http.Handler
}

func (s *Server) ServeOnListener(l net.Listener) error {
	server := http.Server{
		Handler: s.handler,
	}
	return server.Serve(l)
}

// FilterServer rejects requests which don't match one of the specified regular expressions
type FilterServer struct {
	// Only paths that match this regexp will be accepted
	AcceptPaths []*regexp.Regexp
	// Paths that match this regexp will be rejected, even if they match the above
	RejectPaths []*regexp.Regexp
	// Hosts are required to match this list of regexp
	AcceptHosts []*regexp.Regexp
	// Methods that match this regexp are rejected
	RejectMethods []*regexp.Regexp
	// The delegate to call to handle accepted requests.
	delegate http.Handler
}

func matchesRegexp(str string, regexps []*regexp.Regexp) bool {
	for _, re := range regexps {
		if re.MatchString(str) {
			klog.Infof("%v matched %s", str, re)
			return true
		}
	}
	return false
}

func (f *FilterServer) accept(method, path, host string) bool {
	if matchesRegexp(path, f.RejectPaths) {
		return false
	}
	if matchesRegexp(method, f.RejectMethods) {
		return false
	}
	if matchesRegexp(path, f.AcceptPaths) && matchesRegexp(host, f.AcceptHosts) {
		return true
	}
	return false
}

func newFileHandler(prefix, base string) http.Handler {
	return http.StripPrefix(prefix, http.FileServer(http.Dir(base)))
}

type responder struct{}

func (r *responder) Error(err error) {
	klog.Errorf("Error while proxying request: %v", err)
	// http.Error(w, err.Error(), http.StatusInternalServerError)
}

func (r *responder) Object(foo int, obj runtime.Object) {
	// klog.Errorf("Error while proxying request: %v", err)
	// http.Error(w, err.Error(), http.StatusInternalServerError)
}

func NewServer(filebase string, apiProxyPrefix string, staticPrefix string, filter *FilterServer, cfg *rest.Config, keepalive time.Duration, appendLocationPath bool) (*Server, error) {
	transportConfig, err := cfg.TransportConfig()
	if err != nil {
		return nil, err
	}
	tlsConfig, err := transport.TLSConfigFor(transportConfig)
	proxyTransport, err := createProxyTransport(tlsConfig, cfg.Proxy)

	if err != nil {
		return nil, err
	}

	// Kubernetes cluster server address
	location, err := url.Parse(cfg.Host)

	if err != nil {
		return nil, err
	}

	proxyHandler, err := newProxyHandler(apiProxyPrefix, location, proxyTransport, &responder{}, tlsConfig, cfg.Proxy)
	proxyHandler = stripLeaveSlash(apiProxyPrefix, proxyHandler)

	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle(apiProxyPrefix, proxyHandler)
	if filebase != "" {
		// Require user to explicitly request this behavior rather than
		// serving their working directory by default.
		mux.Handle(staticPrefix, newFileHandler(staticPrefix, filebase))
	}
	return &Server{handler: mux}, nil
}

func newProxyHandler(apiProxyPrefix string, location *url.URL, proxyTransport http.RoundTripper,
	responder registryrest.Responder, tlsConfig *tls.Config, Proxy func(*http.Request) (*url.URL, error)) (http.Handler, error) {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		// Retain RawQuery in location because upgrading the request will use it.
		// See https://github.com/karmada-io/karmada/issues/1618#issuecomment-1103793290 for more info.
		location.RawQuery = req.URL.RawQuery

		upgradeDialer := NewUpgradeDialerWithConfig(UpgradeDialerWithConfig{
			TLS:        tlsConfig.Clone(),
			Proxier:    Proxy,
			PingPeriod: time.Second * 5,
			// Header:     ParseProxyHeaders(cluster.Spec.ProxyHeader),
		})

		handler := NewUpgradeAwareHandler(location, proxyTransport, false, httpstream.IsUpgradeRequest(req), proxyutil.NewErrorResponder(responder))
		handler.UpgradeDialer = upgradeDialer
		handler.ServeHTTP(rw, req, apiProxyPrefix)
	}), nil
}

// NewThrottledUpgradeAwareProxyHandler creates a new proxy handler with a default flush interval. Responder is required for returning
// errors to the caller.
func NewThrottledUpgradeAwareProxyHandler(location *url.URL, transport http.RoundTripper, wrapTransport, upgradeRequired bool, responder registryrest.Responder) *proxy.UpgradeAwareHandler {
	return proxy.NewUpgradeAwareHandler(location, transport, wrapTransport, upgradeRequired, proxy.NewErrorResponder(responder))
}

func createProxyTransport(tlsConfig *tls.Config, Proxy func(*http.Request) (*url.URL, error)) (*http.Transport, error) {
	var proxyDialerFn utilnet.DialFunc
	trans := utilnet.SetTransportDefaults(&http.Transport{
		DialContext:     proxyDialerFn,
		TLSClientConfig: tlsConfig.Clone(),
	})

	trans.Proxy = Proxy

	return trans, nil
}

func ParseProxyHeaders(proxyHeaders map[string]string) http.Header {
	if len(proxyHeaders) == 0 {
		return nil
	}

	header := http.Header{}
	for headerKey, headerValues := range proxyHeaders {
		values := strings.Split(headerValues, ",")
		header[headerKey] = values
	}
	return header
}

func skipGroup(group string) bool {
	switch group {
	case user.AllAuthenticated, user.AllUnauthenticated:
		return true
	default:
		return false
	}
}

// like http.StripPrefix, but always leaves an initial slash. (so that our
// regexps will work.)
func stripLeaveSlash(prefix string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		p := strings.TrimPrefix(req.URL.Path, prefix)
		if len(p) >= len(req.URL.Path) {
			http.NotFound(w, req)
			return
		}
		if len(p) > 0 && p[:1] != "/" {
			p = "/" + p
		}
		req.URL.Path = p
		h.ServeHTTP(w, req)
	})
}
