package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOverlayJS_toggleDefaultOff(t *testing.T) {
	t.Parallel()
	assert.Contains(t, overlayJS, "enabled=false")
}

func TestOverlayJS_containsRequiredBindings(t *testing.T) {
	t.Parallel()
	bindings := []string{"esDispatch", "esDelete", "esEdit", "esToggle"}
	for _, b := range bindings {
		assert.Contains(t, overlayJS, b, "JS must reference binding %s", b)
	}
}

func TestOverlayJS_persistsToLocalStorage(t *testing.T) {
	t.Parallel()
	assert.Contains(t, overlayJS, "localStorage")
	assert.Contains(t, overlayJS, "es_notes_")
	assert.Contains(t, overlayJS, "es_enabled")
}

func TestOverlayJS_dispatchTracking(t *testing.T) {
	t.Parallel()
	assert.Contains(t, overlayJS, "dispatchedAt")
	assert.Contains(t, overlayJS, "editedAt")
}

func TestParseDeleteEvent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		input  string
		wantID string
		wantOk bool
	}{
		{"valid", `{"id":"es_123_abc"}`, "es_123_abc", true},
		{"empty id", `{"id":""}`, "", false},
		{"missing id", `{}`, "", false},
		{"invalid json", `not json`, "", false},
		{"extra fields", `{"id":"es_1","x":9}`, "es_1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseDeleteEvent([]byte(tt.input))
			assert.Equal(t, tt.wantOk, ok)
			if ok {
				assert.Equal(t, tt.wantID, got.ID)
			}
		})
	}
}

func TestParseEditEvent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		wantID   string
		wantNote string
		wantOk   bool
	}{
		{"valid", `{"id":"es_1","note":"hello"}`, "es_1", "hello", true},
		{"empty id", `{"id":"","note":"x"}`, "", "", false},
		{"missing note", `{"id":"es_1"}`, "es_1", "", true},
		{"invalid json", `{broken`, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseEditEvent([]byte(tt.input))
			assert.Equal(t, tt.wantOk, ok)
			if ok {
				assert.Equal(t, tt.wantID, got.ID)
				assert.Equal(t, tt.wantNote, got.Note)
			}
		})
	}
}
