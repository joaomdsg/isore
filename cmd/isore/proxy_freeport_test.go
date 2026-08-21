package main

import (
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// startOnFreePort must skip a port already held by another isore instance and
// serve on the next free one, so concurrent per-project instances don't collide.
func TestStartOnFreePortSkipsOccupied(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = occupied.Close() })
	taken := occupied.Addr().(*net.TCPAddr).Port

	h := &proxyHolder{}
	base, port, err := h.startOnFreePort(htmlBackend(t), "127.0.0.1", taken, freshStore(t), []byte(overlayJS), 20*time.Millisecond)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.http.Close() })

	require.NotEqual(t, taken, port, "must not bind the occupied port")
	require.Greater(t, port, taken, "must scan upward from the requested port")
	require.Contains(t, getBody(t, base+"/"), "hi")
}

// When the requested port is free, it is used as-is (no needless scanning).
func TestStartOnFreePortUsesRequestedWhenFree(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	free := probe.Addr().(*net.TCPAddr).Port
	require.NoError(t, probe.Close()) // release it so startOnFreePort can bind it

	h := &proxyHolder{}
	base, port, err := h.startOnFreePort(htmlBackend(t), "127.0.0.1", free, freshStore(t), []byte(overlayJS), 20*time.Millisecond)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.http.Close() })

	require.Equal(t, free, port)
	require.Contains(t, getBody(t, base+"/"), "hi")
}

// A non-EADDRINUSE error must surface immediately, not be masked by scanning.
func TestStartOnFreePortSurfacesOtherErrors(t *testing.T) {
	target, _ := url.Parse("http://127.0.0.1:3000")
	h := &proxyHolder{}
	// An unresolvable host triggers a bind error that is not EADDRINUSE.
	_, _, err := h.startOnFreePort(target, "no.such.host.invalid", 4400, freshStore(t), []byte(overlayJS), 20*time.Millisecond)
	require.Error(t, err)
}
