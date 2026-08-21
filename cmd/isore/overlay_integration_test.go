package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/joaomdsg/isore/internal/browser"
	"github.com/joaomdsg/isore/internal/notes"
	"github.com/stretchr/testify/require"
)

// overlayPage drives the real overlay JS in a headless browser.
type overlayPage struct {
	t *testing.T
	d *browser.Driver
}

func newOverlayPage(t *testing.T) *overlayPage {
	t.Helper()
	chrome := browser.FindChrome()
	if chrome == "" {
		t.Skip("no chromium/chrome binary")
	}
	d, err := browser.Launch(context.Background(), chrome)
	require.NoError(t, err)
	t.Cleanup(d.Close)
	require.NoError(t, d.Navigate("about:blank"))
	_, err = d.Eval(overlayJS)
	require.NoError(t, err)
	return &overlayPage{t: t, d: d}
}

func (p *overlayPage) eval(expr string) json.RawMessage {
	p.t.Helper()
	out, err := p.d.Eval(expr)
	require.NoError(p.t, err, "eval %s", expr)
	return out
}

func (p *overlayPage) evalBool(expr string) bool {
	var v bool
	require.NoError(p.t, json.Unmarshal(p.eval(expr), &v), "expr %s", expr)
	return v
}

func (p *overlayPage) evalString(expr string) string {
	var v string
	require.NoError(p.t, json.Unmarshal(p.eval(expr), &v), "expr %s", expr)
	return v
}

// shadow query helper injected once.
const helpers = `window.__q=function(sel){return document.querySelector('[data-es-root]').shadowRoot.querySelector(sel);};true`

func (p *overlayPage) enable() {
	p.eval(helpers)
	// The redesign folds the old enable/disable switch into idle↔point: click
	// the launcher (or ⌥I) to wake the overlay instead of a separate toggle.
	p.eval(`__q('[data-es=launcher]').click();true`)
	require.True(p.t, p.evalBool(`window.__es.enabled()`), "overlay must be enabled")
}

func (p *overlayPage) addNote(js string) {
	p.eval(`window.__es.notes().push(` + js + `);window.__es.render();true`)
}

func (p *overlayPage) clickBody() {
	p.eval(`document.body.click();true`)
}

func (p *overlayPage) inlineCardOpen() bool {
	return p.evalBool(`!!__q('[data-es=note]')`)
}

func (p *overlayPage) closeInlineCard() {
	p.eval(`__q('[data-es=note] [data-es=cancel]').click();true`)
}

func TestOverlayFreezesWhileAgentWorking(t *testing.T) {
	p := newOverlayPage(t)
	p.enable()

	// Positive control: annotating works when idle.
	p.clickBody()
	require.True(t, p.inlineCardOpen(), "click must open the note card when idle")
	p.closeInlineCard()

	// A note the agent picked up freezes the whole overlay.
	p.addNote(`{id:'n1',selector:'body',note:'x',url:location.href,createdAt:1,editedAt:1,dispatchedAt:1,agentStatus:'working'}`)
	p.clickBody()
	require.False(t, p.inlineCardOpen(), "click must not annotate while agent works")
	require.True(t, p.evalBool(`__q('[data-es=dispatch]').disabled`), "dispatch must be frozen")

	// mark_fixed thaws it.
	p.eval(`(function(){var n=window.__es.notes()[0];n.agentStatus='fixed';n.fixedAt=2;window.__es.render();})();true`)
	require.False(t, p.evalBool(`window.__es.frozen()`), "overlay must thaw once fixed")
	p.clickBody()
	require.True(t, p.inlineCardOpen(), "annotating must resume once fixed")
	p.closeInlineCard()
}

func TestOverlayDispatchDisabledWithNothingToDispatch(t *testing.T) {
	p := newOverlayPage(t)
	p.enable()

	// A note already dispatched, fixed, and unedited since: nothing to dispatch.
	// (fixedAt matters: an outstanding, not-yet-fixed dispatch freezes the whole
	// overlay under the new 'dispatched' state, which is covered separately.)
	p.addNote(`{id:'n1',selector:'body',note:'x',url:location.href,createdAt:1,editedAt:1,dispatchedAt:5,fixedAt:6}`)
	require.True(t, p.evalBool(`__q('[data-es=dispatch]').disabled`), "dispatch must disable with 0 undispatched items")

	// A fresh note enables it.
	p.addNote(`{id:'n2',selector:'body',note:'y',url:location.href,createdAt:6,editedAt:6,dispatchedAt:0}`)
	require.False(t, p.evalBool(`__q('[data-es=dispatch]').disabled`), "dispatch must enable with a pending item")
}

func TestOverlayFreezesOnceDispatchedEvenBeforeTheAgentPicksItUp(t *testing.T) {
	p := newOverlayPage(t)
	p.enable()

	// Dispatched but no agentStatus yet: the agent hasn't called mark_working.
	p.addNote(`{id:'n1',selector:'body',note:'x',url:location.href,createdAt:1,editedAt:1,dispatchedAt:1}`)
	p.clickBody()
	require.False(t, p.inlineCardOpen(), "click must not annotate once dispatched, even pre-pickup")
	require.True(t, p.evalBool(`__q('[data-es=dispatch]').disabled`), "dispatch must freeze on dispatch, not just on pickup")
	require.True(t, p.evalBool(`document.querySelector('[data-es=badge]').getAttribute('data-es-state')==='dispatched'`),
		"badge must show the distinct 'dispatched' color, not fall back to 'pending'")

	// mark_fixed thaws it, same as the working case.
	p.eval(`(function(){var n=window.__es.notes()[0];n.fixedAt=2;window.__es.render();})();true`)
	require.False(t, p.evalBool(`window.__es.frozen()`), "overlay must thaw once fixed")
}

func TestClickingDispatchFreezesTheOverlayImmediatelyNotAfterTheCelebrationTimeout(t *testing.T) {
	p := newOverlayPage(t)
	p.enable()
	p.addNote(`{id:'n1',selector:'body',note:'x',url:location.href,createdAt:1,editedAt:1,dispatchedAt:0}`)

	p.eval(`__q('[data-es=dispatch]').click();true`)

	require.True(t, p.evalBool(`__q('[data-es=dispatch]').disabled`),
		"dispatch must freeze the instant Dispatch is clicked, not after any celebratory timeout")
	p.clickBody()
	require.False(t, p.inlineCardOpen(), "annotation must be blocked immediately on dispatch")
}

func TestSelectorOfCountsNthOfTypeAmongSameTagSiblingsOnly(t *testing.T) {
	p := newOverlayPage(t)
	p.enable()

	// A non-div sibling (header) precedes the div.card siblings, so the
	// second one is the 2nd DIV child of body but the 3rd child overall.
	// selectorOf must count nth-of-type among same-tag siblings only, or the
	// generated selector (e.g. div.card:nth-of-type(3)) won't match the
	// element it was computed from.
	p.eval(`document.body.insertAdjacentHTML('afterbegin',
		'<header></header><div class="card" id="a"></div><div class="card" id="b"><h2>target</h2></div>');true`)

	p.eval(`document.getElementById('b').querySelector('h2').click();true`)
	require.True(t, p.inlineCardOpen(), "clicking the target must open the annotate card")
	p.eval(`__q('[data-es=in]').value='note text';__q('[data-es=add]').click();true`)

	selector := string(p.eval(`window.__es.notes()[0].selector`))
	matches := p.evalBool(`document.querySelector(` + selector + `)===document.getElementById('b').querySelector('h2')`)
	require.True(t, matches, "generated selector %s must resolve back to the clicked element", selector)
}

// TestNudgeSaveDispatchesImmediately covers the risk-kill obligation: saving
// a nudge on a fixed note must POST the full note (with the nudge appended to
// the thread, DispatchedAt left unbumped so it stays pending) to
// /__isore/dispatch right away, before "Dispatch corrections" is ever clicked,
// so a pending nudge is never only client-side state.
func TestNudgeSaveDispatchesImmediately(t *testing.T) {
	chrome := browser.FindChrome()
	if chrome == "" {
		t.Skip("no chromium/chrome binary")
	}
	st := freshStore(t)
	holder := &proxyHolder{}
	base, err := holder.start(htmlBackend(t), "127.0.0.1:0", st, []byte(overlayJS), 20*time.Millisecond)
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.http.Close() })

	// Seed the fixed note in the STORE, not client-side: on a proxied page the
	// SSE onopen fires reconcile(), and /__isore/notes is authoritative — a
	// note only pushed into window.__es.notes() is wiped before we can click.
	// Its URL must be the page we load: the overlay renders only the notes
	// belonging to the current page.
	require.NoError(t, st.Merge([]notes.Note{{
		ID: "n1", Selector: "body", Note: "x", URL: base + "/", CreatedAt: 1, EditedAt: 1,
		DispatchedAt: 1, FixedAt: 2,
		Thread: []notes.ThreadEntry{{Kind: notes.EntrySummary, Text: "old summary", At: 2}},
	}}))

	d, err := browser.Launch(context.Background(), chrome)
	require.NoError(t, err)
	t.Cleanup(d.Close)
	require.NoError(t, d.Navigate(base+"/"))
	p := &overlayPage{t: t, d: d}
	p.eval(helpers)
	p.enable()

	// Wait for reconcile() to deliver the seeded note and render its ✎ button.
	require.Eventually(t, func() bool {
		return p.evalBool(`!!__q('.es-nudge')`)
	}, 3*time.Second, 50*time.Millisecond, "seeded fixed note must render a nudge button")
	p.eval(`__q('.es-nudge').click();true`)
	p.eval(`__q('[data-es=nudgebox] textarea').value='new summary';` +
		`__q('[data-es=nudge-add]').click();true`)

	require.Eventually(t, func() bool {
		all, err := st.Load()
		if err != nil || len(all) == 0 || len(all[0].Thread) == 0 {
			return false
		}
		last := all[0].Thread[len(all[0].Thread)-1]
		return last.Kind == "nudge" && last.Text == "new summary" && all[0].DispatchedAt == 1
	}, 3*time.Second, 50*time.Millisecond, "saved nudge must reach the store via /__isore/dispatch immediately, still pending")
}

// TestNudgeTextSurvivesThePeriodicRender is the regression for "the nudge
// input updates every second and won't let me change the text". It pins three
// things: (1) the periodic ensure() tick does NOT re-render when the overlay
// is intact, so it can't touch the textarea; (2) even a forced render leaves
// the focused textarea alone; (3) a render while unfocused restores the typed
// draft rather than snapping back to the original summary.
func TestNudgeTextSurvivesThePeriodicRender(t *testing.T) {
	p := newOverlayPage(t)
	p.enable()

	p.addNote(`{id:'n1',selector:'body',note:'x',url:location.href,createdAt:1,editedAt:1,` +
		`dispatchedAt:1,fixedAt:2,thread:[{kind:'summary',text:'old summary',at:2}]}`)
	p.eval(`__q('.es-nudge').click();true`)

	// Focus and type into the nudge textarea, firing the same input event a
	// keystroke would.
	p.eval(`(function(){var ta=__q('[data-es=nudgebox] textarea');ta.focus();` +
		`ta.value='my new idea';ta.dispatchEvent(new Event('input',{bubbles:true}));})();true`)

	// (1) The real periodic tick: with the overlay intact it must not render,
	// so the textarea is untouched.
	p.eval(`window.__es.ensure();true`)
	require.Equal(t, "my new idea", p.evalString(`__q('[data-es=nudgebox] textarea').value`),
		"the periodic ensure() tick must not re-render an intact overlay")

	// (2) A forced render (as an SSE diff would trigger) must leave the focused
	// textarea alone.
	p.eval(`window.__es.render();true`)
	require.Equal(t, "my new idea", p.evalString(`__q('[data-es=nudgebox] textarea').value`),
		"a render must not clobber the focused nudge textarea")

	// (3) A render while unfocused must restore the typed draft, not the old
	// summary.
	p.eval(`__q('[data-es=nudgebox] textarea').blur();true`)
	p.eval(`window.__es.render();true`)
	require.Equal(t, "my new idea", p.evalString(`__q('[data-es=nudgebox] textarea').value`),
		"an unfocused render must restore the draft, not the old summary")
}

// TestFreezePredicateIncludesInFlightNudge covers the frozen contract: a note
// whose nudge has been dispatched back to the agent (the thread's last entry is
// a nudge and DispatchedAt is past it) must freeze the whole overlay even
// though the note itself is already fixedAt-stamped and not "working".
func TestFreezePredicateIncludesInFlightNudge(t *testing.T) {
	p := newOverlayPage(t)
	p.enable()

	p.addNote(`{id:'n1',selector:'body',note:'x',url:location.href,createdAt:1,editedAt:1,` +
		`dispatchedAt:4,fixedAt:2,thread:[{kind:'summary',text:'old',at:2},{kind:'nudge',base:'old',text:'new',at:3}]}`)

	require.True(t, p.evalBool(`window.__es.frozen()`), "an in-flight nudge must freeze the overlay")
	p.clickBody()
	require.False(t, p.inlineCardOpen(), "click must not annotate while a correction batch is in flight")
}

func TestCloseButtonActuallyDeletesTheNote(t *testing.T) {
	p := newOverlayPage(t)
	p.enable()
	p.addNote(`{id:'n1',selector:'body',note:'x',url:location.href,createdAt:1,editedAt:1,dispatchedAt:0}`)

	p.eval(`document.querySelector('[data-es=badge]').dispatchEvent(new Event('mouseenter'));true`)
	require.True(t, p.evalBool(`!!__q('[data-es=popover]')`), "hover must open the popover")

	p.eval(`__q('[data-es=pop-del]').click();true`)
	p.eval(`__q('[data-es=del-confirm]').click();true`)

	require.True(t, p.evalBool(`window.__es.notes().length===0`), "confirming Close must actually remove the note")
}
