// Original local research probe, 2026-09-30.
// Run: go run examples/glance-redirect-probe.go
// Synthetic hosts are mapped to httptest listeners; no DNS, proxy, real token,
// production service, upstream application or TLS-bypass setting is involved.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sync/atomic"
	"time"
)

var errOriginChanged = errors.New("redirect rejected: origin changed")

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var destinationHits atomic.Int64
	received := make(chan http.Header, 1)
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destinationHits.Add(1)
		received <- r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"synthetic-only"}`)
	}))
	defer destination.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://destination.invalid/final", http.StatusFound)
	}))
	defer origin.Close()

	// Keep unrelated URL hostnames while sending every TCP connection to one
	// of these two loopback-only listeners. Unknown addresses fail locally.
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			var localAddress string
			switch address {
			case "origin.invalid:80":
				localAddress = origin.Listener.Addr().String()
			case "destination.invalid:80":
				localAddress = destination.Listener.Addr().String()
			default:
				return nil, fmt.Errorf("unmapped synthetic address: %s", address)
			}
			return (&net.Dialer{}).DialContext(ctx, network, localAddress)
		},
	}
	defer transport.CloseIdleConnections()

	newRequest := func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodGet, "http://origin.invalid/start", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer demo-only")
		req.Header.Set("x-api-key", "demo-only")
		return req, nil
	}

	req, err := newRequest()
	if err != nil {
		return err
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	_, readErr := io.Copy(io.Discard, response.Body)
	closeErr := response.Body.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	if destinationHits.Load() != 1 {
		return fmt.Errorf("default policy: destination hits = %d, want 1", destinationHits.Load())
	}
	headers := <-received
	if headers.Get("Authorization") != "" || headers.Get("x-api-key") != "demo-only" {
		return errors.New("default policy: unexpected fake-header forwarding result")
	}
	fmt.Printf("go=%s\n", runtime.Version())
	fmt.Println("default: destination_hits=1 authorization_present=false x_api_key_present=true")

	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		initial := via[0].URL
		if req.URL.Scheme != initial.Scheme || req.URL.Host != initial.Host {
			return errOriginChanged
		}
		return nil
	}
	req, err = newRequest()
	if err != nil {
		return err
	}
	response, err = client.Do(req)
	if err == nil {
		response.Body.Close()
		return errors.New("same-origin policy unexpectedly followed cross-origin redirect")
	}
	if !errors.Is(err, errOriginChanged) {
		return err
	}
	if destinationHits.Load() != 1 {
		return errors.New("same-origin policy contacted destination before rejecting redirect")
	}
	fmt.Printf("same-origin: expected_error=%s\n", err)
	fmt.Println("same-origin: additional_destination_hits=0")
	return nil
}
