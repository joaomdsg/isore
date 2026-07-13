package serve

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// MarkWorking flags a note as picked up so the user's overlay badge changes.
func (h *Handlers) MarkWorking(_ context.Context, id string) error {
	return h.store.MarkWorking(id)
}

// Reload asks the proxy to refresh connected browsers. It only works in proxy
// mode; ReloadURL is empty under the chromedp harness.
func (h *Handlers) Reload(ctx context.Context) error {
	if h.ReloadURL == "" {
		return errors.New("no proxy connected: page reload is only available in proxy mode")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.ReloadURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("proxy reload failed: %s", resp.Status)
	}
	return nil
}
