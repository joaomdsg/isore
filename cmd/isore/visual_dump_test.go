package main

// Visual-verification harness: dumps a PNG of each overlay state to
// /tmp/isore-visual/ for design review. Runs only when ISORE_VISUAL_DUMP=1
// so it never slows CI.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVisualDumpOfAllStates(t *testing.T) {
	if os.Getenv("ISORE_VISUAL_DUMP") != "1" {
		t.Skip("set ISORE_VISUAL_DUMP=1 to dump state screenshots")
	}
	dir := "/tmp/isore-visual"
	require.NoError(t, os.MkdirAll(dir, 0o755))
	p := newOverlayPage(t)
	p.eval(helpers)
	shot := func(name string) {
		png, err := p.d.Screenshot("")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name+".png"), png, 0o644))
	}

	shot("1-idle")

	p.eval(`__q('[data-es=launcher]').click();true`)
	shot("2-point")

	p.eval(`window.__es.notes().push(
		{id:'a',selector:'button.btn-primary',label:'Save',note:'Sits below the baseline — nudge it up 3px.',url:'x',createdAt:1,editedAt:1,dispatchedAt:0},
		{id:'b',selector:'h3.card-title',label:'Title',note:'Heading is crammed against the card edge.',url:'x',createdAt:1,editedAt:1,dispatchedAt:0},
		{id:'c',selector:'p.help-text',label:'Help',note:'This helper text is too faint to read.',url:'x',createdAt:1,editedAt:1,dispatchedAt:0}
	);window.__es.render();true`)
	shot("3-queue")

	p.eval(`window.__es.notes().forEach(function(n){n.dispatchedAt=2;n.agentStatus='working';});window.__es.render();true`)
	shot("4-handoff")

	p.eval(`var s={a:'Snapped to the baseline, aligned to the 8px grid.',b:'Bumped card padding to 24px.',c:'Raised contrast to WCAG AA.'};
		window.__es.notes().forEach(function(n){n.fixedAt=3;n.agentStatus='fixed';n.thread=[{kind:'summary',text:s[n.id],at:3}];});window.__es.render();true`)
	shot("5-fixed")

	p.eval(`__q('.es-nudge').click();true`)
	shot("6a-nudge-editor")

	p.eval(`__q('[data-es=nudgebox] textarea').value='Snapped to the baseline, on the 8px grid, and bolded.';
		__q('[data-es=nudge-add]').click();true`)
	shot("6b-tracked-diff")
}
