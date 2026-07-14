package main

import (
	"context"
	"sync"

	"github.com/joaomdsg/isore/internal/browser"
)

// installHint tells the agent how to get a drivable browser onto the user's
// machine, per platform. Surfaced verbatim in start_proxy's message.
const installHint = "Warn the user: screenshots and the browser_* tools need Chromium or Chrome. " +
	"Suggest they install one — Arch: sudo pacman -S chromium; Debian/Ubuntu: sudo apt install chromium; " +
	"Fedora: sudo dnf install chromium; macOS: brew install --cask chromium — then call start_proxy again."

// userBrowser owns the headed, chromedp-driven Chromium the user annotates
// in. One instance per MCP server; the driver is reused across start_proxy
// restarts. Function fields exist so tests stub the world out.
type userBrowser struct {
	mu       sync.Mutex
	d        *browser.Driver
	find     func() string
	launch   func(ctx context.Context, chrome string) (*browser.Driver, error)
	navigate func(d *browser.Driver, url string) error
	fallback func(url string)
}

func newUserBrowser() *userBrowser {
	return &userBrowser{
		find:     browser.FindChrome,
		launch:   browser.LaunchVisible,
		navigate: (*browser.Driver).Navigate,
		fallback: func(url string) { openBrowser(url) },
	}
}

// open shows rawURL in a driven headed browser, launching one on first call.
// When no drivable browser is available (or launch/navigate fails) it falls
// back to the user's default browser and returns a non-empty warning for the
// agent; the returned driver is nil in that case.
func (u *userBrowser) open(rawURL string) (warning string, d *browser.Driver) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.d == nil {
		chrome := u.find()
		if chrome == "" {
			u.fallback(rawURL)
			return "WARNING: no Chromium or Chrome binary found — opened the user's default browser instead, " +
				"which isore cannot drive. " + installHint, nil
		}
		nd, err := u.launch(context.Background(), chrome)
		if err != nil {
			u.fallback(rawURL)
			return "WARNING: found " + chrome + " but launching it failed (" + err.Error() + ") — " +
				"opened the user's default browser instead, which isore cannot drive. " + installHint, nil
		}
		u.d = nd
	}
	if err := u.navigate(u.d, rawURL); err != nil {
		// The user likely closed the window; relaunch once next call.
		u.d.Close()
		u.d = nil
		u.fallback(rawURL)
		return "WARNING: the driven browser is gone (" + err.Error() + ") — opened the user's default " +
			"browser instead. Call start_proxy again to relaunch the driven one.", nil
	}
	return "", u.d
}
