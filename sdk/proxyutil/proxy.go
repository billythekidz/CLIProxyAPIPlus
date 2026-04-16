package proxyutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/net/proxy"
)

// Mode describes how a proxy setting should be interpreted.
type Mode int

const (
	// ModeInherit means no explicit proxy behavior was configured.
	ModeInherit Mode = iota
	// ModeDirect means outbound requests must bypass proxies explicitly.
	ModeDirect
	// ModeProxy means a concrete proxy URL was configured.
	ModeProxy
	// ModeInvalid means the proxy setting is present but malformed or unsupported.
	ModeInvalid
)

// Setting is the normalized interpretation of a proxy configuration value.
type Setting struct {
	Raw  string
	Mode Mode
	URL  *url.URL
}

var dockerResolver = &net.Resolver{
	PreferGo: true,
	Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, "127.0.0.11:53")
	},
}

type dockerDirectDialer struct{}

func (d *dockerDirectDialer) Dial(network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Resolver: dockerResolver}
	return dialer.Dial(network, addr)
}

// Parse normalizes a proxy configuration value into inherit, direct, or proxy modes.
func Parse(raw string) (Setting, error) {
	trimmed := strings.TrimSpace(raw)
	setting := Setting{Raw: trimmed}

	if trimmed == "" {
		setting.Mode = ModeInherit
		return setting, nil
	}

	if strings.EqualFold(trimmed, "direct") || strings.EqualFold(trimmed, "none") {
		setting.Mode = ModeDirect
		return setting, nil
	}

	parsedURL, errParse := url.Parse(trimmed)
	if errParse != nil {
		setting.Mode = ModeInvalid
		return setting, fmt.Errorf("parse proxy URL failed: %w", errParse)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		setting.Mode = ModeInvalid
		return setting, fmt.Errorf("proxy URL missing scheme/host")
	}

	switch parsedURL.Scheme {
	case "socks5", "socks5h", "http", "https":
		setting.Mode = ModeProxy
		setting.URL = parsedURL
		return setting, nil
	default:
		setting.Mode = ModeInvalid
		return setting, fmt.Errorf("unsupported proxy scheme: %s", parsedURL.Scheme)
	}
}

func cloneDefaultTransport() *http.Transport {
	if transport, ok := http.DefaultTransport.(*http.Transport); ok && transport != nil {
		return transport.Clone()
	}
	return &http.Transport{}
}

// NewDirectTransport returns a transport that bypasses environment proxies.
func NewDirectTransport() *http.Transport {
	clone := cloneDefaultTransport()
	clone.Proxy = nil
	return clone
}

// isInternalHost attempts to automatically detect internal IP addresses and
// docker compose service names (which lack top-level domains).
func isInternalHost(host string) bool {
	if host == "localhost" {
		return true
	}
	// Check if it's a bare hostname (no dots), typical for Docker services or local names
	if !strings.Contains(host, ".") && host != "" {
		return true
	}
	// Check if it's a loopback or private IP
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return true
		}
	}
	return false
}

// BuildHTTPTransport constructs an HTTP transport for the provided proxy setting.
func BuildHTTPTransport(raw string) (*http.Transport, Mode, error) {
	setting, errParse := Parse(raw)
	if errParse != nil {
		return nil, setting.Mode, errParse
	}

	switch setting.Mode {
	case ModeInherit:
		return nil, setting.Mode, nil
	case ModeDirect:
		return NewDirectTransport(), setting.Mode, nil
	case ModeProxy:
		if setting.URL.Scheme == "socks5" || setting.URL.Scheme == "socks5h" {
			var proxyAuth *proxy.Auth
			if setting.URL.User != nil {
				username := setting.URL.User.Username()
				password, _ := setting.URL.User.Password()
				proxyAuth = &proxy.Auth{User: username, Password: password}
			}
			proxyHost := setting.URL.Host
			if strings.HasPrefix(proxyHost, "warp:") || proxyHost == "warp" {
				proxyHost = strings.Replace(proxyHost, "warp", "127.0.0.1", 1)
			}
			dialer, errSOCKS5 := proxy.SOCKS5("tcp", proxyHost, proxyAuth, &dockerDirectDialer{})
			if errSOCKS5 != nil {
				return nil, setting.Mode, fmt.Errorf("create SOCKS5 dialer failed: %w", errSOCKS5)
			}
			
			perHost := proxy.NewPerHost(dialer, &dockerDirectDialer{})
			perHost.AddFromString("localhost")
			perHost.AddFromString("127.0.0.1")
			
			noProxyEnv := os.Getenv("NO_PROXY")
			if noProxyEnv == "" {
				noProxyEnv = os.Getenv("no_proxy")
			}
			for _, h := range strings.Split(noProxyEnv, ",") {
				if t := strings.TrimSpace(h); t != "" {
					perHost.AddFromString(t)
				}
			}

			transport := cloneDefaultTransport()
			transport.Proxy = nil
			transport.DialContext = func(_ context.Context, network, addr string) (net.Conn, error) {
				host, _, err := net.SplitHostPort(addr)
				if err != nil {
					host = addr
				}
				if isInternalHost(host) {
					var dd dockerDirectDialer
					return dd.Dial(network, addr)
				}
				return perHost.Dial(network, addr)
			}
			return transport, setting.Mode, nil
		}
		transport := cloneDefaultTransport()
		transport.Proxy = func(req *http.Request) (*url.URL, error) {
			noProxyEnv := os.Getenv("NO_PROXY")
			if noProxyEnv == "" {
				noProxyEnv = os.Getenv("no_proxy")
			}
			
			hosts := append(strings.Split(noProxyEnv, ","), "localhost", "127.0.0.1")
			reqHost, _, err := net.SplitHostPort(req.URL.Host)
			if err != nil {
				reqHost = req.URL.Host
			}
			if isInternalHost(reqHost) {
				return nil, nil // Return nil to bypass proxy
			}
			for _, h := range hosts {
				if t := strings.TrimSpace(h); t != "" && (reqHost == t || strings.HasSuffix(reqHost, "."+t)) {
					return nil, nil // Return nil to bypass proxy
				}
			}
			return setting.URL, nil
		}
		return transport, setting.Mode, nil
	default:
		return nil, setting.Mode, nil
	}
}

// BuildDialer constructs a proxy dialer for settings that operate at the connection layer.
func BuildDialer(raw string) (proxy.Dialer, Mode, error) {
	setting, errParse := Parse(raw)
	if errParse != nil {
		return nil, setting.Mode, errParse
	}

	switch setting.Mode {
	case ModeInherit:
		return nil, setting.Mode, nil
	case ModeDirect:
		return proxy.Direct, setting.Mode, nil
	case ModeProxy:
		dialer, errDialer := proxy.FromURL(setting.URL, proxy.Direct)
		if errDialer != nil {
			return nil, setting.Mode, fmt.Errorf("create proxy dialer failed: %w", errDialer)
		}
		return dialer, setting.Mode, nil
	default:
		return nil, setting.Mode, nil
	}
}
