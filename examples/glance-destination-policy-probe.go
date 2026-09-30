// Original source-boundary experiment, 2026-09-30. Standard library only.
// Synthetic hosts are mapped to loopback httptest listeners, without DNS,
// environment proxies, credentials or any real dashboard/API.
// Run: go run examples/glance-destination-policy-probe.go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"runtime"
	"sync/atomic"
	"time"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var errRouteChanged = errors.New("redirect rejected: approved route changed")

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func statusURL(id string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", errors.New("invalid metric identifier")
	}
	return "http://metrics.invalid/v1/status/" + id, nil
}

func fetch(client *http.Client, target string) error {
	resp, err := client.Get(target)
	if err != nil {
		return err
	}
	_, readErr := io.Copy(io.Discard, resp.Body)
	closeErr := resp.Body.Close()
	if readErr != nil {
		return readErr
	}
	return closeErr
}

func run() error {
	var internalHits, adminHits atomic.Int64
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		internalHits.Add(1)
		fmt.Fprintln(w, `{"synthetic":"internal-only"}`)
	}))
	defer internal.Close()
	metrics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/index":
			fmt.Fprintln(w, `{"next":"http://internal.invalid/admin"}`)
		case "/v1/status/redirect-path":
			http.Redirect(w, r, "/admin", http.StatusFound)
		case "/admin":
			adminHits.Add(1)
			fmt.Fprintln(w, `{"synthetic":"admin-route"}`)
		default:
			fmt.Fprintln(w, `{"status":"synthetic-only"}`)
		}
	}))
	defer metrics.Close()
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			var localAddress string
			switch address {
			case "metrics.invalid:80":
				localAddress = metrics.Listener.Addr().String()
			case "internal.invalid:80":
				localAddress = internal.Listener.Addr().String()
			default:
				return nil, fmt.Errorf("unmapped synthetic address: %s", address)
			}
			return (&net.Dialer{}).DialContext(ctx, network, localAddress)
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	// Simulate an index response controlling the next complete request URL.
	resp, err := client.Get("http://metrics.invalid/v1/index")
	if err != nil {
		return err
	}
	var index struct{ Next string }
	decodeErr := json.NewDecoder(resp.Body).Decode(&index)
	closeErr := resp.Body.Close()
	if decodeErr != nil {
		return decodeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := fetch(client, index.Next); err != nil {
		return err
	}
	if internalHits.Load() != 1 {
		return errors.New("complete response URL did not reach synthetic internal service")
	}
	fmt.Printf("go=%s\n", runtime.Version())
	fmt.Println("response-url: synthetic_internal_hits=1")

	for _, id := range []string{"cpu-01", "../admin", "node?mode=write", "node%2Fadmin", "http://internal.invalid/admin"} {
		_, err := statusURL(id)
		accepted := err == nil
		if accepted != (id == "cpu-01") {
			return fmt.Errorf("unexpected identifier result: %q", id)
		}
		fmt.Printf("bounded-id: %q accepted=%t\n", id, accepted)
	}

	target, err := statusURL("redirect-path")
	if err != nil {
		return err
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		first := via[0].URL
		if req.URL.Scheme != first.Scheme || req.URL.Host != first.Host {
			return errRouteChanged
		}
		return nil
	}
	if err := fetch(client, target); err != nil {
		return err
	}
	if adminHits.Load() != 1 {
		return errors.New("origin-only policy did not reach synthetic admin route")
	}
	fmt.Println("origin-only: same_host_admin_hits=1")

	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		first := via[0].URL
		if req.URL.Scheme != first.Scheme || req.URL.Host != first.Host ||
			req.URL.Path != first.Path || req.URL.RawQuery != first.RawQuery {
			return errRouteChanged
		}
		return nil
	}
	err = fetch(client, target)
	if !errors.Is(err, errRouteChanged) {
		return fmt.Errorf("expected route-policy rejection, got %v", err)
	}
	if adminHits.Load() != 1 {
		return errors.New("route policy contacted admin route before rejecting redirect")
	}
	fmt.Printf("route-policy: expected_error=%s\n", err)
	fmt.Println("route-policy: additional_admin_hits=0")
	return nil
}
