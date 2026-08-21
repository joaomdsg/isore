package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/joaomdsg/isore/internal/browser"
	"github.com/stretchr/testify/require"
)

func stubbedUserBrowser() (*userBrowser, *[]string) {
	calls := &[]string{}
	u := &userBrowser{
		find: func() string { return "/usr/bin/chromium" },
		launch: func(_ context.Context, chrome string) (*browser.Driver, error) {
			*calls = append(*calls, "launch:"+chrome)
			return &browser.Driver{}, nil
		},
		navigate: func(_ *browser.Driver, url string) error {
			*calls = append(*calls, "navigate:"+url)
			return nil
		},
		fallback: func(url string) { *calls = append(*calls, "fallback:"+url) },
	}
	return u, calls
}

func TestUserBrowserOpensDrivenChromium(t *testing.T) {
	u, calls := stubbedUserBrowser()
	warn, d := u.open("http://127.0.0.1:4400")
	require.Empty(t, warn)
	require.NotNil(t, d)
	require.Equal(t, []string{"launch:/usr/bin/chromium", "navigate:http://127.0.0.1:4400"}, *calls)
}

func TestUserBrowserReusesDriverAcrossRestarts(t *testing.T) {
	u, calls := stubbedUserBrowser()
	_, d1 := u.open("http://a")
	warn, d2 := u.open("http://b")
	require.Empty(t, warn)
	require.Same(t, d1, d2)
	require.Equal(t, []string{"launch:/usr/bin/chromium", "navigate:http://a", "navigate:http://b"}, *calls)
}

func TestUserBrowserWarnsWhenNoChromeFound(t *testing.T) {
	u, calls := stubbedUserBrowser()
	u.find = func() string { return "" }
	warn, d := u.open("http://127.0.0.1:4400")
	require.Nil(t, d)
	require.Equal(t, []string{"fallback:http://127.0.0.1:4400"}, *calls, "must open default browser, never launch")
	require.Contains(t, warn, "no Chromium or Chrome")
	require.Contains(t, warn, "install")
}

func TestUserBrowserWarnsWhenLaunchFails(t *testing.T) {
	u, calls := stubbedUserBrowser()
	u.launch = func(context.Context, string) (*browser.Driver, error) {
		return nil, fmt.Errorf("boom")
	}
	warn, d := u.open("http://x")
	require.Nil(t, d)
	require.Contains(t, *calls, "fallback:http://x")
	require.Contains(t, warn, "boom")
}

func TestDriverPoolSeed(t *testing.T) {
	p := &driverPool{}
	d := &browser.Driver{}
	p.seed(d)
	got, release, err := p.get(context.Background())
	require.NoError(t, err)
	require.Same(t, d, got, "seeded driver must be served without launching")
	release()
}

func TestDriverPoolSeedReplacesStaleDriver(t *testing.T) {
	stale := &browser.Driver{}
	p := &driverPool{d: stale}
	visible := &browser.Driver{}
	p.seed(visible)
	got, release, err := p.get(context.Background())
	require.NoError(t, err)
	require.Same(t, visible, got, "the user-visible driver must win over a pre-existing headless one")
	release() // seed below drains in-flight callers, so hold nothing across it

	p.seed(visible) // re-seed on proxy restart must not close the live driver
	got, release2, err := p.get(context.Background())
	require.NoError(t, err)
	require.Same(t, visible, got)
	release2()
}

// A start_proxy that reseeds the pool must not Close the driver a browser_*
// call is still driving: MCP tool calls are not guaranteed to be serialized.
func TestDriverPoolSeedWaitsForInFlightCaller(t *testing.T) {
	inUse := &browser.Driver{}
	p := &driverPool{d: inUse}

	got, release, err := p.get(context.Background())
	require.NoError(t, err)
	require.Same(t, inUse, got)

	seeded := make(chan struct{})
	go func() {
		p.seed(&browser.Driver{})
		close(seeded)
	}()

	select {
	case <-seeded:
		t.Fatal("seed swapped (and closed) the driver while a caller still held it")
	case <-time.After(50 * time.Millisecond):
	}

	release()
	select {
	case <-seeded:
	case <-time.After(2 * time.Second):
		t.Fatal("seed never completed after the in-flight caller released")
	}
}
