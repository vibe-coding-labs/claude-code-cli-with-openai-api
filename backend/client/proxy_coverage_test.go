package client

// Coverage tests for proxy.go — these drive every remaining uncovered branch
// in BuildTransportProxy to reach 100% statement coverage for the client
// package (task #4).
//
// Branch map (from the coverprofile count=0 blocks):
//   - proxy.FromURL error => warn + http.ProxyFromEnvironment fallback
//   - dialCtx closure's proxy.ContextDialer TRUE branch (cd.DialContext)
//   - dialCtx closure's FALSE branch (dialer.Dial) — reachable only by
//     injecting a dialer that implements Dial without DialContext: the real
//     x/net socks5 dialer (internal/socks.Dialer) DOES implement DialContext,
//     so in production the true branch always wins.
//
// Strategy: the package-level seam socksDialerFromURL lets tests inject a
// dialer implementing proxy.ContextDialer to hit the true branch, or one
// implementing only Dial to hit the false branch; an unknown-scheme URL makes
// FromURL itself return the error that drives the fallback.

import (
	"context"
	"errors"
	"net"
	"net/url"
	"testing"

	"golang.org/x/net/proxy"
)

// fakeContextDialer implements both Dial and DialContext so the seam can inject
// it and BuildTransportProxy's ContextDialer type-assertion succeeds.
type fakeContextDialer struct {
	dialed  bool
	dctxErr error
}

func (f *fakeContextDialer) Dial(network, addr string) (net.Conn, error) {
	f.dialed = true
	return nil, net.ErrClosed
}

func (f *fakeContextDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	f.dialed = true
	return nil, f.dctxErr
}

// fakeDialOnly implements only proxy.Dialer (Dial, no DialContext) so the seam
// can inject it and BuildTransportProxy's ContextDialer type-assertion fails,
// driving the closure's dialer.Dial fallback branch — unreachable with the real
// socks5 dialer, which implements DialContext.
type fakeDialOnly struct {
	dialed  bool
	dialErr error
}

func (f *fakeDialOnly) Dial(network, addr string) (net.Conn, error) {
	f.dialed = true
	return nil, f.dialErr
}

// ---------------------------------------------------------------------
// proxy.FromURL error fallback (blocks 103.17-106.4)
// ---------------------------------------------------------------------

func TestBuildTransportProxy_Socks5FromURLErrorFallback(t *testing.T) {
	// An explicit ProxyType forces the socks5 branch even when the URL scheme
	// is "ftp" — url.Parse succeeds on the host, but proxy.FromURL rejects the
	// scheme with "proxy: unknown scheme: ftp", driving the fallback.
	proxyFunc, dialCtx := BuildTransportProxy(ProxyConfig{
		ProxyURL:  "ftp://127.0.0.1:1",
		ProxyType: "socks5",
	})
	if proxyFunc == nil {
		t.Fatal("expected non-nil Proxy func (http.ProxyFromEnvironment fallback)")
	}
	if dialCtx != nil {
		t.Error("expected nil dialCtx on fallback, got a non-nil function")
	}
}

// ---------------------------------------------------------------------
// dialCtx closure — false branch: dialer.Dial (default real socks5 dialer)
// (blocks 111.4-111.37)
// ---------------------------------------------------------------------

func TestBuildTransportProxy_Socks5DialFallbackBranch(t *testing.T) {
	// The real x/net socks5 dialer (internal/socks.Dialer) implements
	// proxy.ContextDialer, so the false branch is unreachable in production.
	// Inject a dialer that implements ONLY Dial to make the type-assertion fail
	// and drive the closure's dialer.Dial branch. Dialing doesn't even need to
	// succeed — the closure's statements have executed either way.
	fake := &fakeDialOnly{dialErr: net.ErrClosed}
	orig := socksDialerFromURL
	defer func() { socksDialerFromURL = orig }()
	socksDialerFromURL = func(u *url.URL, forward proxy.Dialer) (proxy.Dialer, error) {
		return fake, nil
	}

	proxyFunc, dialCtx := BuildTransportProxy(ProxyConfig{
		ProxyURL:  "socks5://127.0.0.1:1",
		ProxyType: "socks5",
	})
	if proxyFunc != nil {
		t.Error("expected nil Proxy func for socks5, got a non-nil function")
	}
	if dialCtx == nil {
		t.Fatal("expected non-nil dialCtx for socks5")
	}
	_, err := dialCtx(context.Background(), "tcp", "example.com:80")
	if !errors.Is(err, net.ErrClosed) {
		t.Errorf("expected the injected Dial error to surface, got %v", err)
	}
	if !fake.dialed {
		t.Error("expected Dial to be invoked on the injected dialer")
	}
}

// ---------------------------------------------------------------------
// dialCtx closure — true branch: cd.DialContext (seam-injected fake)
// (blocks 107.79-108.50 and 108.50-110.5)
// ---------------------------------------------------------------------

func TestBuildTransportProxy_Socks5ContextDialerBranch(t *testing.T) {
	orig := socksDialerFromURL
	defer func() { socksDialerFromURL = orig }()

	fake := &fakeContextDialer{dctxErr: net.ErrClosed}
	socksDialerFromURL = func(u *url.URL, forward proxy.Dialer) (proxy.Dialer, error) {
		return fake, nil
	}

	_, dialCtx := BuildTransportProxy(ProxyConfig{
		ProxyURL:  "socks5://127.0.0.1:1",
		ProxyType: "socks5",
	})
	if dialCtx == nil {
		t.Fatal("expected non-nil dialCtx for socks5")
	}
	_, err := dialCtx(context.Background(), "tcp", "example.com:80")
	if !errors.Is(err, net.ErrClosed) {
		t.Errorf("expected the injected DialContext error to surface, got %v", err)
	}
	if !fake.dialed {
		t.Error("expected DialContext to be invoked on the fake dialer")
	}
}