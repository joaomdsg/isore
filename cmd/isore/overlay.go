package main

const overlayJS = `
(function(){
  if(window.__es){window.__es.ensure();return;}
  var notes=[],dialogOpen=false;
  var filter='all',editingId=null,collapsed=true;

  function isES(el){return !!(el&&el.closest&&(el.closest('[data-es]')||el.closest('[data-es-root]')));}

  var proxyMode=!window.esDispatch;
  function ship(json){
    if(window.esDispatch){window.esDispatch(json);return;}
    fetch('/__isore/dispatch',{method:'POST',headers:{'Content-Type':'application/json'},body:json});
  }
  function shipDelete(noteId){
    if(window.esDelete){window.esDelete(JSON.stringify({id:noteId}));return;}
    fetch('/__isore/delete',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:noteId})});
  }
  function shipComment(noteId,text){
    if(window.esComment){window.esComment(JSON.stringify({id:noteId,text:text}));return;}
    fetch('/__isore/comment',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:noteId,text:text})});
  }
  // applyDiff is additive/update-only: safe for the SSE 'notes' event, which
  // streams only what changed, never the full set.
  function applyDiff(list){
    var touched=false;
    list.forEach(function(c){
      var found=false;
      for(var i=0;i<notes.length;i++){
        if(notes[i].id===c.id){
          notes[i].agentStatus=c.agentStatus;
          notes[i].fixedAt=c.fixedAt;
          notes[i].dispatchedAt=c.dispatchedAt;
          notes[i].comments=c.comments;
          notes[i].thread=c.thread;
          found=true;touched=true;
        }
      }
      if(!found&&c.url===location.href){notes.push(c);touched=true;}
    });
    if(touched){save();render();}
  }
  // applyFull additionally drops any locally-held note that the server no
  // longer has: /__isore/notes returns every note, so absence is authoritative
  // deletion — unlike the SSE diff stream, which never carries "this is everything".
  function applyFull(list){
    applyDiff(list);
    var serverIds={};list.forEach(function(c){serverIds[c.id]=true;});
    var before=notes.length;
    notes=notes.filter(function(n){return n.dispatchedAt===0||serverIds[n.id];});
    if(notes.length!==before){save();render();}
  }
  // reconcile pulls the authoritative note state: SSE only streams diffs, so
  // any gap in the stream (tab closed, proxy restarted) is healed here. It is
  // also how a pending nudge (saved but not yet dispatched) survives a page
  // reload — the correction fields ride along on every note in the full set.
  function reconcile(){
    fetch('/__isore/notes').then(function(r){return r.json();})
      .then(applyFull).catch(function(){});
  }
  var sseLive=false;
  function listen(){
    if(!proxyMode||window.__esSSE)return;
    try{
      var es=new EventSource('/__isore/events');window.__esSSE=es;
      // onopen refires on every auto-reconnect — reconcile each time, since
      // whatever streamed while the connection was down is gone for good.
      es.onopen=function(){sseLive=true;reconcile();render();};
      es.onerror=function(){sseLive=false;render();};
      es.addEventListener('reload',function(){location.reload();});
      es.addEventListener('notes',function(ev){
        try{applyDiff(JSON.parse(ev.data)||[]);}catch(_){}
      });
      es.addEventListener('deleted',function(ev){
        try{
          var d=JSON.parse(ev.data);
          var before=notes.length;
          notes=notes.filter(function(n){return n.id!==d.id;});
          if(notes.length!==before){save();render();}
        }catch(_){}
      });
    }catch(_){}
  }

  function id(){return 'es_'+Date.now()+'_'+Math.random().toString(36).slice(2,6);}

  function esc(s){return(s||'').replace(/[&<>"]/g,function(c){return{'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c];});}

  function selectorOf(el){
    if(!el||el.nodeType!==1)return'body';
    if(el.tagName==='HTML'||el.tagName==='BODY')return el.tagName.toLowerCase();
    if(el.id)return'#'+el.id;
    var parts=[],cur=el,depth=0;
    while(cur&&cur.nodeType===1&&cur.tagName!=='BODY'&&depth<6){
      var s=cur.tagName.toLowerCase();
      if(cur.classList&&cur.classList.length)s+='.'+Array.prototype.slice.call(cur.classList,0,2).join('.');
      var p=cur.parentNode;
      if(p){var same=Array.prototype.filter.call(p.children,function(c){return c.tagName===cur.tagName;});
        if(same.length>1)s+=':nth-of-type('+(same.indexOf(cur)+1)+')';}
      parts.unshift(s);cur=cur.parentNode;depth++;
    }
    return parts.join(' > ')||el.tagName.toLowerCase();
  }

  // ── brand mark & wordmark ──────────────────────────────────
  // One <circle> + one rotated <rect> — no illustration. The mark stands in
  // for the "o" in the wordmark lockup: is·mark·re.
  function markSVG(px,ring){
    return '<svg viewBox="0 0 24 24" width="'+px+'" height="'+px+'" fill="none" xmlns="http://www.w3.org/2000/svg">'+
      '<circle cx="12" cy="12" r="9" stroke="'+(ring||'#ececf1')+'" stroke-width="2.8"/>'+
      '<rect x="9.4" y="9.4" width="5.2" height="5.2" rx="1.5" fill="#f2555a" transform="rotate(20 12 12)"/></svg>';
  }
  function wordmarkHTML(){
    return '<span class="es-wordmark">'+
      '<span class="es-wm-is">is</span>'+
      '<span class="es-wm-mark">'+markSVG(16)+'</span>'+
      '<span class="es-wm-re">re</span></span>';
  }

  // ── stylesheets ────────────────────────────────────────────
  // Chrome lives in a shadow root so page CSS (e.g. global button rules)
  // cannot restyle it; badges and outlines attach to page elements, so their
  // rules must stay in the light DOM.
  function injectShadowStyles(){
    if(root.querySelector('#es-css'))return;
    var st=document.createElement('style');st.id='es-css';
    st.textContent=[
      '@font-face{font-family:"Space Grotesk";font-weight:600;font-display:swap;',
        'src:url(/__isore/fonts/space-grotesk-600.woff2) format("woff2")}',
      '@font-face{font-family:"Instrument Sans";font-weight:400;font-display:swap;',
        'src:url(/__isore/fonts/instrument-sans-400.woff2) format("woff2")}',
      '@font-face{font-family:"Instrument Sans";font-weight:500;font-display:swap;',
        'src:url(/__isore/fonts/instrument-sans-500.woff2) format("woff2")}',
      '@font-face{font-family:"Instrument Sans";font-weight:600;font-display:swap;',
        'src:url(/__isore/fonts/instrument-sans-600.woff2) format("woff2")}',
      '@font-face{font-family:"JetBrains Mono";font-weight:400;font-display:swap;',
        'src:url(/__isore/fonts/jetbrains-mono-400.woff2) format("woff2")}',
      '@font-face{font-family:"JetBrains Mono";font-weight:500;font-display:swap;',
        'src:url(/__isore/fonts/jetbrains-mono-500.woff2) format("woff2")}',
      ':host{all:initial;',
        '--iso-head:"Space Grotesk",ui-sans-serif,system-ui,sans-serif;',
        '--iso-body:"Instrument Sans",ui-sans-serif,system-ui,sans-serif;',
        '--iso-mono:"JetBrains Mono",ui-monospace,SFMono-Regular,Menlo,monospace;',
        '--iso-accent:#6d5efc;--iso-eyesore:#f2555a;--iso-working:#f5a623;--iso-fixed:#3ecf8e}',
      '*{box-sizing:border-box}',
      '@keyframes es-fade{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:translateY(0)}}',
      '@keyframes es-pop{from{opacity:0;transform:scale(.5)}to{opacity:1;transform:scale(1)}}',
      '@keyframes es-pulse{0%,100%{box-shadow:0 0 0 0 rgba(245,166,35,0)}50%{box-shadow:0 0 0 7px rgba(245,166,35,.18)}}',
      '@keyframes es-blink{50%{opacity:.35}}',
      '',
      '[data-es=hi]{position:fixed;pointer-events:none;z-index:2147483646;',
        'border:2px solid rgba(242,85,90,.65);background:rgba(242,85,90,.07);',
        'display:none;border-radius:10px;box-shadow:0 0 0 4px rgba(242,85,90,.1);',
        'transition:all .12s ease-out}',
      '[data-es=inspect]{position:fixed;pointer-events:none;z-index:2147483646;display:none;',
        'background:#131317;border:1px solid rgba(255,255,255,.1);border-radius:6px;',
        'padding:3px 8px;font:10px/1.5 var(--iso-mono);white-space:nowrap}',
      '[data-es=inspect] .es-in-sel{color:#f2828a}',
      '[data-es=inspect] .es-in-dim{color:#9a9aa4;margin-left:6px}',
      '[data-es=flagpill]{position:fixed;pointer-events:none;z-index:2147483646;display:none;',
        'background:var(--iso-accent);color:#fff;border-radius:999px;padding:3px 8px 3px 10px;',
        'font:500 11px/1.6 var(--iso-body);align-items:center;gap:6px}',
      '.es-kbd{display:inline-block;font:9px/1.6 var(--iso-mono);padding:0 5px;border-radius:4px;',
        'background:rgba(255,255,255,.2);color:#fff;margin-left:5px}',
      '.es-kbd.es-kbd-dark{background:rgba(255,255,255,.06);border:1px solid rgba(255,255,255,.1);color:#9a9aa4;margin-left:6px}',
      '',
      '[data-es=launcher]{position:fixed;right:20px;bottom:20px;z-index:2147483647;',
        'width:50px;height:50px;border-radius:15px;cursor:pointer;border:1px solid rgba(255,255,255,.12);padding:0;',
        'background:linear-gradient(180deg,#1e1e26,#131318);',
        'box-shadow:0 12px 30px -8px rgba(0,0,0,.5),0 6px 18px -8px rgba(109,94,252,.45),inset 0 1px 0 rgba(255,255,255,.1);',
        'display:flex;align-items:center;justify-content:center;',
        'transition:transform .15s ease,box-shadow .15s ease;',
        'animation:es-fade .18s ease-out}',
      '[data-es=launcher]:hover{transform:scale(1.05)}',
      '[data-es=launcher] .es-launcher-badge{position:absolute;top:-4px;right:-4px;',
        'min-width:16px;height:16px;border-radius:999px;background:var(--iso-eyesore);color:#fff;',
        'font:650 9.5px/16px var(--iso-body);text-align:center;padding:0 3px;box-shadow:0 0 0 2px #131318}',
      '',
      '[data-es=toolbar]{position:fixed;right:20px;bottom:20px;z-index:2147483647;width:290px;',
        'border-radius:15px;border:1px solid rgba(255,255,255,.14);',
        'background:radial-gradient(135% 95% at 50% -12%,rgba(124,111,255,.16),transparent 55%),',
          'linear-gradient(180deg,rgba(37,37,47,.97),rgba(12,12,16,.98));',
        'box-shadow:0 32px 74px -20px rgba(0,0,0,.85),0 14px 36px -14px rgba(109,94,252,.42),',
          'inset 0 1px 0 rgba(255,255,255,.16),inset 0 -24px 44px -32px rgba(0,0,0,.7);',
        'backdrop-filter:blur(18px) saturate(1.3);padding:14px;',
        'font:13px/1.4 var(--iso-body);color:#ededf2;animation:es-fade .18s ease-out}',
      '.es-toolbar-row{display:flex;align-items:center;gap:8px;padding-bottom:10px;',
        'border-bottom:1px solid rgba(255,255,255,.07);margin-bottom:10px}',
      '.es-brand{display:flex;align-items:center;gap:2px;flex:1;min-width:0}',
      '.es-wordmark{display:flex;align-items:baseline;font:600 18px/1 var(--iso-head);',
        'letter-spacing:-.045em;color:#eeeef2;white-space:nowrap}',
      '.es-wm-is{margin-right:-.07em}',
      '.es-wm-mark{display:inline-flex;margin:0 -.12em;transform:translateY(.12em)}',
      '.es-wm-mark svg{display:block}',
      '.es-hide{flex-shrink:0;background:none;border:none;cursor:pointer;color:#6f6f79;',
        'font:14px/1 var(--iso-body);padding:2px 4px;border-radius:4px}',
      '.es-hide:hover{color:#c2c2ca;background:rgba(255,255,255,.06)}',
      '.es-hint{font:12.5px/1.5 var(--iso-body);color:#9a9aa4;padding:4px 2px 6px}',
      '',
      '.es-filters{display:flex;gap:6px;margin-bottom:10px}',
      '.es-chip{display:flex;align-items:center;gap:5px;border-radius:8px;padding:4px 9px;',
        'font:11px/1.6 var(--iso-body);cursor:pointer;border:1px solid rgba(255,255,255,.07);',
        'background:transparent;color:#8f8f99}',
      '.es-chip.es-chip-on{background:rgba(255,255,255,.11);border-color:rgba(255,255,255,.2);',
        'color:#f2f2f5;box-shadow:inset 0 1px 0 rgba(255,255,255,.1)}',
      '.es-chip-badge{border-radius:999px;padding:0 6px;font:600 10px/1.5 var(--iso-mono);',
        'background:rgba(255,255,255,.09);color:#8f8f99}',
      '.es-chip-badge.es-badge-open{background:rgba(242,85,90,.18);color:#ff8f94}',
      '.es-chip-badge.es-badge-amber{background:rgba(245,166,35,.18);color:#f5b53f;animation:es-pulse 1.2s ease-in-out infinite}',
      '.es-chip-badge.es-badge-fixed{background:rgba(62,207,142,.18);color:#5fe0a6}',
      '',
      '[data-es=list]{list-style:none;margin:0 0 10px;padding:0;max-height:236px;overflow-y:auto;',
        'scrollbar-width:thin;scrollbar-color:rgba(255,255,255,.16) transparent}',
      '[data-es=list]::-webkit-scrollbar{width:7px}',
      '[data-es=list]::-webkit-scrollbar-thumb{background:rgba(255,255,255,.14);border-radius:99px;border:3px solid transparent}',
      '[data-es=list]::-webkit-scrollbar-thumb:hover{background:rgba(255,255,255,.26)}',
      '.es-row{padding:8px 4px;border-bottom:1px solid rgba(255,255,255,.07);cursor:pointer}',
      '.es-row:last-child{border-bottom:none}',
      '.es-row-main{display:flex;align-items:flex-start;gap:7px}',
      '.es-dot{flex-shrink:0;width:8px;height:8px;border-radius:50%;margin-top:4px;background:var(--iso-eyesore)}',
      '.es-dot.es-dot-amber{background:var(--iso-working);animation:es-pulse 1.2s ease-in-out infinite}',
      '.es-dot.es-dot-fixed{background:var(--iso-fixed)}',
      '.es-row-body{min-width:0;flex:1}',
      '.es-row-note{color:#d2d2d9;overflow:hidden;text-overflow:ellipsis;display:-webkit-box;',
        '-webkit-line-clamp:2;-webkit-box-orient:vertical}',
      '.es-row.es-row-fixed .es-row-note{color:#7a7a84;text-decoration:line-through}',
      '.es-row-meta{font:10px/1.6 var(--iso-mono);color:#6f6f79;overflow:hidden;',
        'text-overflow:ellipsis;white-space:nowrap;margin-top:1px}',
      '',
      '.es-summary{margin-top:6px;background:#191920;border:1px solid rgba(255,255,255,.08);',
        'border-radius:10px;padding:7px 9px;font:11.5px/1.5 var(--iso-body)}',
      '.es-summary-row{display:flex;align-items:flex-start;gap:6px}',
      '.es-summary-text{color:var(--iso-fixed);flex:1;min-width:0}',
      '.es-summary-correcting{color:#f5b53f}',
      '.es-nudge{flex-shrink:0;width:22px;height:22px;border-radius:6px;border:none;',
        'background:transparent;color:#b9b0ff;cursor:pointer;font:13px/22px var(--iso-body);text-align:center;padding:0}',
      '.es-nudge:hover{background:rgba(255,255,255,.08)}',
      '.es-diff{display:flex;align-items:flex-start;gap:6px;margin-top:2px}',
      '.es-diff-arrows{flex-shrink:0;display:flex;flex-direction:column;font-size:10px;line-height:1.3}',
      '.es-diff-arrows .es-up{color:var(--iso-eyesore)}',
      '.es-diff-arrows .es-down{color:var(--iso-fixed)}',
      '.es-diff-body{min-width:0}',
      '.es-thread{display:flex;flex-direction:column;gap:7px}',
      '.es-thread-act{margin-top:2px}',
      '.es-tok-same{color:#c9c9d0}',
      '.es-tok-del{color:#f2828a;text-decoration:line-through}',
      '.es-tok-add{color:var(--iso-fixed);background:rgba(62,207,142,.14);border-radius:3px}',
      '',
      '[data-es=nudgebox] textarea,[data-es=notefield]{width:100%;box-sizing:border-box;',
        'background:#0e0e12;border:1px solid var(--iso-accent);border-radius:8px;',
        'box-shadow:0 0 0 3px rgba(109,94,252,.14);color:#eeeef2;',
        'font:13.5px/1.5 var(--iso-body);padding:8px 9px;resize:vertical;outline:none}',
      '[data-es=nudgebox] textarea{font-size:12.5px;margin-top:6px}',
      '.es-btnrow{display:flex;gap:6px;justify-content:flex-end;margin-top:8px}',
      '.es-ghost{background:#22222a;border:1px solid #33333d;color:#c2c2ca;border-radius:8px;',
        'padding:6px 11px;font:600 11.5px/1 var(--iso-body);cursor:pointer}',
      '.es-ghost:hover{filter:brightness(1.15)}',
      '.es-accentbtn{background:var(--iso-accent);border:none;color:#fff;border-radius:8px;',
        'padding:6px 11px;font:600 11.5px/1 var(--iso-body);cursor:pointer;display:flex;align-items:center;gap:4px}',
      '.es-accentbtn:hover{filter:brightness(1.1)}',
      '',
      '[data-es=dispatch]{border:none;border-radius:9px;cursor:pointer;width:100%;',
        'font:600 12.5px/1 var(--iso-body);color:#fff;background:var(--iso-accent);',
        'padding:10px 0;display:flex;align-items:center;justify-content:center;gap:6px;transition:filter .15s ease}',
      '[data-es=dispatch]:not(:disabled):hover{filter:brightness(1.1)}',
      '[data-es=dispatch]:disabled{background:#26262e;color:#7a7a84;cursor:default}',
      '[data-es=dispatch].es-done{background:#1c5c3f;color:#8ff0c0}',
      '',
      '[data-es=popover]{position:fixed;z-index:2147483645;background:#1e1e1e;color:#e0e0e0;',
        'padding:12px 16px;border-radius:10px;max-width:340px;min-width:220px;',
        'font:13px/1.5 var(--iso-body);box-shadow:0 12px 40px rgba(0,0,0,.5),0 0 0 1px rgba(255,255,255,.06);',
        'backdrop-filter:blur(8px);animation:es-fade .15s ease-out}',
      '.es-pop-note{margin-bottom:8px;white-space:pre-wrap;word-break:break-word;color:#e8e8e8}',
      '.es-pop-comment{margin-bottom:6px;padding-left:8px;border-left:2px solid #444;',
        'white-space:pre-wrap;word-break:break-word;color:#bbb;font-size:12px}',
      '.es-pop-meta{font-size:11px;color:#666;margin-bottom:8px;font-family:var(--iso-mono)}',
      '.es-pop-agent{font-size:12px;color:#7fd6a4;margin-bottom:8px}',
      '.es-pop-actions{display:flex;gap:6px;justify-content:flex-end}',
      '.es-btn{padding:5px 12px;font-size:12px;border-radius:5px;cursor:pointer;border:none;',
        'font:600 12px/1 var(--iso-body);transition:filter .15s ease}',
      '.es-btn:hover{filter:brightness(1.15)}',
      '.es-btn-edit{background:#2a2a2a;color:#bbb;border:1px solid #444}',
      '.es-btn-del{background:#24242c;color:#b9b9c4;border:1px solid #3a3a44}',
      '.es-btn-cancel{background:#2a2a2a;color:#bbb;border:1px solid #444}',
      '.es-btn-confirm{background:#c62828;color:#fff}',
      '.es-btn-save{background:var(--iso-accent);color:#fff}',
      '',
      '[data-es=note]{position:fixed;z-index:2147483647;width:302px;border-radius:13px;',
        'background:radial-gradient(135% 95% at 50% -12%,rgba(124,111,255,.16),transparent 55%),',
          'linear-gradient(180deg,rgba(37,37,47,.97),rgba(12,12,16,.98));',
        'border:1px solid rgba(255,255,255,.14);',
        'box-shadow:0 32px 74px -20px rgba(0,0,0,.85),0 14px 36px -14px rgba(109,94,252,.42),',
          'inset 0 1px 0 rgba(255,255,255,.16);backdrop-filter:blur(18px) saturate(1.3);',
        'padding:12px;font:13px/1.4 var(--iso-body);color:#ededf2;animation:es-fade .15s ease-out}',
      '.es-note-ctx{display:flex;align-items:center;gap:8px;padding-bottom:10px;',
        'border-bottom:1px solid rgba(255,255,255,.07);margin-bottom:10px}',
      '.es-note-thumb{flex-shrink:0;width:44px;height:30px;border-radius:5px;',
        'background:repeating-linear-gradient(45deg,#2a2a33,#2a2a33 4px,#33333d 4px,#33333d 8px)}',
      '.es-note-sel{font:11px/1.4 var(--iso-mono);color:#c2c2ca;overflow:hidden;',
        'text-overflow:ellipsis;white-space:nowrap;flex:1;min-width:0}',
      '.es-dim{font:10px/1.6 var(--iso-mono);color:#9a9aa4;background:rgba(255,255,255,.06);',
        'border-radius:4px;padding:1px 5px;flex-shrink:0}',
      '.es-captured{font:10px/1.6 var(--iso-body);color:var(--iso-fixed);flex-shrink:0;white-space:nowrap}',
      '[data-es=note] textarea{width:100%;box-sizing:border-box;background:#0e0e12;',
        'border:1px solid var(--iso-accent);border-radius:9px;box-shadow:0 0 0 3px rgba(109,94,252,.14);',
        'color:#eeeef2;font:13.5px/1.5 var(--iso-body);padding:9px 10px;outline:none;resize:vertical}',
      '.es-note-foot{display:flex;align-items:center;justify-content:space-between;margin-top:10px}',
      '.es-note-esc{font:11px/1.4 var(--iso-body);color:#6f6f79}',
      '.es-caret{display:inline-block;width:1px;height:1em;background:#eeeef2;',
        'vertical-align:-.15em;animation:es-blink 1s step-end infinite}'
    ].join('');
    root.appendChild(st);
  }

  function injectLightStyles(){
    if(document.getElementById('es-css-light'))return;
    var st=document.createElement('style');st.id='es-css-light';
    st.textContent=[
      '@keyframes es-scaleIn{from{opacity:0;transform:scale(0.5)}to{opacity:1;transform:scale(1)}}',
      '@keyframes es-lpulse{0%,100%{box-shadow:0 0 0 0 rgba(245,166,35,0)}50%{box-shadow:0 0 0 7px rgba(245,166,35,.35)}}',
      '@keyframes es-flash{0%,100%{outline:2px solid transparent}',
        '50%{outline:3px solid rgba(242,85,90,.8)}}',
      '[data-es=badge]{position:fixed;min-width:22px;height:22px;border-radius:999px;padding:0 5px;',
        'background:#f2555a;color:#fff;display:flex;align-items:center;justify-content:center;',
        'font:600 11px/1 var(--iso-body,system-ui);cursor:pointer;z-index:2147483640;',
        'box-shadow:0 1px 2px rgba(0,0,0,.25),0 4px 12px -2px rgba(242,85,90,.35);',
        'border:2px solid #fff;animation:es-scaleIn .2s ease-out;',
        'transition:transform .15s ease;user-select:none}',
      '[data-es=badge]:hover{transform:scale(1.15)}',
      '[data-es=badge][data-es-state=dispatched]{background:#f5a623;animation-name:es-scaleIn,es-lpulse;',
        'animation-duration:.2s,1.2s;animation-iteration-count:1,infinite}',
      '[data-es=badge][data-es-state=working]{background:#f5a623;animation-name:es-scaleIn,es-lpulse;',
        'animation-duration:.2s,1.2s;animation-iteration-count:1,infinite}',
      '[data-es=badge][data-es-state=fixed]{background:#3ecf8e}',
      '[data-es-marked]{outline:2px solid rgba(242,85,90,.4)!important;outline-offset:3px;',
        'border-radius:8px;background:rgba(242,85,90,.05);',
        'transition:outline-color .18s ease,outline-offset .18s ease}',
      '[data-es-marked=dispatched]{outline-color:rgba(245,166,35,.4)!important;background:transparent}',
      '[data-es-marked=working]{outline-color:rgba(245,166,35,.4)!important;background:transparent}',
      '[data-es-marked=fixed]{outline-color:rgba(62,207,142,.4)!important;background:transparent}',
      '[data-es-marked]:hover{outline-width:2.5px}'
    ].join('');
    document.head.appendChild(st);
  }

  // ── persistence ────────────────────────────────────────────
  function storageKey(){return 'es_notes_'+location.href;}
  function save(){
    try{localStorage.setItem(storageKey(),JSON.stringify(notes));
        localStorage.setItem('es_collapsed',collapsed?'1':'0');}catch(_){}
  }
  function load(){
    try{var s=localStorage.getItem(storageKey());if(s)notes=JSON.parse(s)||[];
        var c=localStorage.getItem('es_collapsed');collapsed=c===null?true:c==='1';}catch(_){notes=[];}
  }

  // ── DOM scaffold ───────────────────────────────────────────
  var host=null,root=null,hi=null,inspect=null,flagpill=null,toolbar=null,dispatchBtn=null;
  var badgeLayer=null,launcher=null,toolbarBody=null;
  // editingDraft keeps the in-progress nudge text in state so it survives a
  // render (the 1s ensure() tick, or an SSE diff arriving mid-edit) instead of
  // living only in the DOM node that render() replaces. editingJustOpened makes
  // select-all happen once, on open, not on every render.
  var editingDraft='',editingJustOpened=false,everRendered=false;

  // ensure re-injects any overlay scaffolding the host page tore out (an SPA
  // re-render or route change can remove our nodes, and there is no event for
  // that — hence the periodic tick that calls it). It renders ONLY when it
  // actually (re)built something or on first run; otherwise it leaves the DOM
  // untouched. All content updates render on their own event (SSE diff, user
  // action) — there is deliberately no every-second re-render.
  function ensure(){
    if(!document.body)return;
    var built=false;
    if(!host||!document.body.contains(host)){
      host=document.createElement('div');host.setAttribute('data-es-root','');
      root=host.attachShadow({mode:'open'});
      document.body.appendChild(host);
      built=true;
    }
    if(!badgeLayer||!document.body.contains(badgeLayer)){
      badgeLayer=document.createElement('div');badgeLayer.setAttribute('data-es','badge-layer');
      badgeLayer.style.cssText='position:fixed;top:0;left:0;width:0;height:0;overflow:visible;z-index:2147483640;pointer-events:none';
      document.body.appendChild(badgeLayer);
      built=true;
    }
    injectShadowStyles();
    injectLightStyles();
    if(!hi||!root.contains(hi)){hi=document.createElement('div');hi.setAttribute('data-es','hi');root.appendChild(hi);built=true;}
    if(!inspect||!root.contains(inspect)){inspect=document.createElement('div');inspect.setAttribute('data-es','inspect');root.appendChild(inspect);built=true;}
    if(!flagpill||!root.contains(flagpill)){
      flagpill=document.createElement('div');flagpill.setAttribute('data-es','flagpill');
      flagpill.innerHTML='click to flag<span class="es-kbd">C</span>';
      root.appendChild(flagpill);built=true;
    }
    if(!launcher||!root.contains(launcher)){
      launcher=document.createElement('button');launcher.setAttribute('data-es','launcher');
      launcher.type='button';launcher.title='isore';launcher.innerHTML=markSVG(26);
      launcher.addEventListener('click',function(ev){
        ev.preventDefault();ev.stopPropagation();wake();
      },true);
      root.appendChild(launcher);built=true;
    }
    if(!toolbar||!root.contains(toolbar)){
      built=true;
      toolbar=document.createElement('div');toolbar.setAttribute('data-es','toolbar');

      var row=document.createElement('div');row.className='es-toolbar-row';
      var brand=document.createElement('div');brand.className='es-brand';brand.innerHTML=wordmarkHTML();
      var akbd=document.createElement('span');akbd.className='es-kbd es-kbd-dark';akbd.textContent='⌥I';
      var hideBtn=document.createElement('button');hideBtn.className='es-hide';hideBtn.type='button';
      hideBtn.textContent='⌄';hideBtn.title='Hide';
      hideBtn.addEventListener('click',function(ev){ev.preventDefault();ev.stopPropagation();sleep();},true);
      row.appendChild(brand);row.appendChild(akbd);row.appendChild(hideBtn);

      toolbarBody=document.createElement('div');toolbarBody.setAttribute('data-es','body');

      toolbar.appendChild(row);toolbar.appendChild(toolbarBody);
      root.appendChild(toolbar);
    }
    if(built||!everRendered){everRendered=true;render();}
  }

  // ── wake / sleep (idle ↔ point) ─────────────────────────
  function wake(){collapsed=false;save();render();}
  function sleep(){collapsed=true;dialogOpen=false;editingId=null;save();render();}

  // ── derived state ──────────────────────────────────────────
  function anyWorking(){
    for(var i=0;i<notes.length;i++){if(notes[i].dispatchedAt>0&&!notes[i].fixedAt)return true;}
    return false;
  }
  function anyCorrecting(){
    for(var i=0;i<notes.length;i++){if(inFlightNudge(notes[i]))return true;}
    return false;
  }
  // isFrozen covers a dispatched-but-unfixed note (sent, agent hasn't picked
  // it up yet, or is actively working it) and an in-flight correction batch
  // — the whole annotator freezes for both, mirroring the real product.
  function isFrozen(){return anyWorking()||anyCorrecting();}
  // ── note thread helpers (single source of open/nudge state) ─────────
  function lastEntry(n){var t=n.thread||[];return t.length?t[t.length-1]:null;}
  function latestSummary(n){var t=n.thread||[];for(var i=t.length-1;i>=0;i--){if(t[i].kind==='summary')return t[i].text;}return '';}
  // A pending nudge is the latest entry, a nudge saved but NOT yet dispatched
  // (its At is newer than the last hand-off). An in-flight nudge is one already
  // dispatched, so the agent is re-working it.
  function hasPendingCorrection(n){var e=lastEntry(n);return !!e&&e.kind==='nudge'&&(n.dispatchedAt||0)<e.at;}
  function inFlightNudge(n){var e=lastEntry(n);return !!e&&e.kind==='nudge'&&(n.dispatchedAt||0)>=e.at;}
  function pendingNudgeCount(){
    var c=0;notes.forEach(function(n){if(hasPendingCorrection(n))c++;});return c;
  }
  function countEdited(){return notes.filter(function(n){return n.dispatchedAt===0||n.editedAt>n.dispatchedAt;}).length;}
  // Open = never fixed (or reopened by a re-dispatch, which zeroes fixedAt), or
  // fixed with the latest thread entry still a nudge (pending or in-flight).
  // Mirrors notes.Open on the server so the open count stays 1 from dispatch
  // until the agent's next summary lands. Counts live only in the chip badges.
  function isOpen(n){if(!n.fixedAt)return true;var e=lastEntry(n);return !!e&&e.kind!=='summary';}
  function counts(){
    var open=0,fixed=0;
    notes.forEach(function(n){if(isOpen(n))open++;else fixed++;});
    return {all:notes.length,open:open,fixed:fixed};
  }

  // ── word-level LCS diff (tracked-change rendering) ──────────
  function diffWords(a,b){
    var A=(a||'').split(/\s+/).filter(Boolean),B=(b||'').split(/\s+/).filter(Boolean);
    var n=A.length,m=B.length,dp=[];
    for(var i=0;i<=n;i++)dp.push(new Array(m+1).fill(0));
    for(i=n-1;i>=0;i--)for(var j=m-1;j>=0;j--)dp[i][j]=A[i]===B[j]?dp[i+1][j+1]+1:Math.max(dp[i+1][j],dp[i][j+1]);
    var runs=[];i=0;var j=0;
    while(i<n&&j<m){
      if(A[i]===B[j]){runs.push({t:'same',w:A[i]});i++;j++;}
      else if(dp[i+1][j]>=dp[i][j+1]){runs.push({t:'del',w:A[i]});i++;}
      else{runs.push({t:'add',w:B[j]});j++;}
    }
    while(i<n){runs.push({t:'del',w:A[i]});i++;}
    while(j<m){runs.push({t:'add',w:B[j]});j++;}
    return runs;
  }
  function diffHTML(a,b){
    return diffWords(a,b).map(function(r){
      var cls=r.t==='same'?'es-tok-same':(r.t==='del'?'es-tok-del':'es-tok-add');
      return '<span class="'+cls+'">'+esc(r.w)+'</span>';
    }).join(' ');
  }

  // ── toolbar body (point hint / queue inbox / fixed+corrections) ─────────
  function render(){
    // While the user is typing in the nudge editor, do NOT tear the toolbar
    // down — the 1s ensure() tick (and any stray render) would otherwise wipe
    // the focused textarea and its caret every time. Skip only when that exact
    // textarea holds focus; the next render after they blur/save/cancel (which
    // clears editingId) rebuilds normally.
    if(editingId&&toolbarBody&&root){
      var open=toolbarBody.querySelector('[data-es=nudgebox] textarea');
      if(open&&root.activeElement===open)return;
    }
    applyCollapsed();
    if(!toolbarBody)return;
    toolbarBody.innerHTML='';
    if(notes.length===0){
      var hint=document.createElement('div');hint.className='es-hint';
      hint.textContent='Hover an element, then click to flag it.';
      toolbarBody.appendChild(hint);
    }else{
      toolbarBody.appendChild(buildFilters());
      toolbarBody.appendChild(buildList());
    }
    toolbarBody.appendChild(buildFooter());
    if(hi&&(collapsed||isFrozen()||dialogOpen))hi.style.display='none';
    if(inspect&&(collapsed||dialogOpen))inspect.style.display='none';
    if(flagpill&&(collapsed||dialogOpen))flagpill.style.display='none';
    renderBadges();
  }

  function buildFilters(){
    var c=counts(),wrap=document.createElement('div');wrap.className='es-filters';
    [['all','All',c.all,'neutral'],['open','Open',c.open,anyWorking()||anyCorrecting()?'amber':'open'],['fixed','Fixed',c.fixed,'fixed']]
      .forEach(function(f){
        var chip=document.createElement('button');chip.type='button';
        chip.className='es-chip'+(filter===f[0]?' es-chip-on':'');
        var badgeCls=f[3]==='neutral'?'':(f[3]==='amber'?'es-badge-amber':(f[3]==='open'?'es-badge-open':'es-badge-fixed'));
        chip.innerHTML=f[1]+' <span class="es-chip-badge '+badgeCls+'">'+f[2]+'</span>';
        chip.addEventListener('click',function(ev){ev.preventDefault();ev.stopPropagation();filter=f[0];render();},true);
        wrap.appendChild(chip);
      });
    return wrap;
  }

  function visibleNotes(){
    return notes.filter(function(n){
      if(filter==='open')return isOpen(n);
      if(filter==='fixed')return !isOpen(n);
      return true;
    });
  }

  function buildList(){
    var ul=document.createElement('div');ul.setAttribute('data-es','list');
    visibleNotes().forEach(function(n){ul.appendChild(buildRow(n));});
    return ul;
  }

  function buildRow(n){
    var row=document.createElement('div');
    var settled=!isOpen(n);
    row.className='es-row'+(settled?' es-row-fixed':'');
    var main=document.createElement('div');main.className='es-row-main';
    var dot=document.createElement('div');
    dot.className='es-dot'+((n.agentStatus==='working'||(n.dispatchedAt>0&&!n.fixedAt)||inFlightNudge(n))?' es-dot-amber':(settled?' es-dot-fixed':''));
    var body=document.createElement('div');body.className='es-row-body';
    var txt=document.createElement('div');txt.className='es-row-note';txt.textContent=n.note;
    var meta=document.createElement('div');meta.className='es-row-meta';meta.textContent=n.selector+(n.dim?' · '+n.dim:'');
    body.appendChild(txt);body.appendChild(meta);
    if((n.thread||[]).length||editingId===n.id)body.appendChild(buildThread(n));
    main.appendChild(dot);main.appendChild(body);row.appendChild(main);
    row.addEventListener('click',function(ev){
      if(isES(ev.target)&&ev.target.closest('[data-es=nudgebox],.es-nudge,textarea,button'))return;
      try{
        var el=document.querySelector(n.selector);if(!el)return;
        el.scrollIntoView({behavior:'smooth',block:'center'});
        el.style.animation='es-flash 0.6s ease 3';
        setTimeout(function(){el.style.animation='';},2000);
      }catch(_){}
    },true);
    return row;
  }

  // ── note thread (history of summaries + nudges) + editor ────
  // Renders the whole thread oldest-first: each agent summary as a ✓ line,
  // each user nudge as a base→edited tracked-change diff. The trailing block is
  // the live affordance: the open editor, an "agent re-working" indicator while
  // a nudge is in flight, or the ✎ to nudge the latest summary when settled.
  function buildThread(n){
    var card=document.createElement('div');card.className='es-summary';
    var t=n.thread||[];
    var hist=document.createElement('div');hist.className='es-thread';
    var html='';
    // While the editor is open the pending nudge is still just a draft (not yet
    // a thread entry), so the last real entry is the summary being revised —
    // render every committed entry here.
    for(var i=0;i<t.length;i++){
      var e=t[i];
      if(e.kind==='summary'){
        html+='<div class="es-summary-row"><span class="es-summary-text">✓ '+esc(e.text)+'</span></div>';
      }else{
        html+='<div class="es-diff"><div class="es-diff-arrows"><span class="es-up">←</span><span class="es-down">→</span></div>'+
          '<div class="es-diff-body">'+diffHTML(e.base||'',e.text)+'</div></div>';
      }
    }
    hist.innerHTML=html;
    if(html)card.appendChild(hist);

    if(editingId===n.id){
      var box=document.createElement('div');box.setAttribute('data-es','nudgebox');
      box.innerHTML='<textarea rows="3" placeholder="Edit the summary…"></textarea>'+
        '<div class="es-btnrow">'+
        '<button class="es-ghost" data-es="nudge-cancel">Cancel</button>'+
        '<button class="es-accentbtn" data-es="nudge-add">Add<span class="es-kbd">⌘⏎</span></button></div>';
      var ta=box.querySelector('textarea');ta.value=editingDraft;
      // Persist keystrokes into state so any render (SSE diff mid-edit, a
      // scaffold rebuild) restores what was typed instead of resetting it.
      ta.addEventListener('input',function(){editingDraft=ta.value;},true);
      // Select-all only when the box first opens; on any later render keep the
      // caret at the end so restored focus doesn't clobber the user's position.
      setTimeout(function(){
        ta.focus();
        if(editingJustOpened){ta.select();editingJustOpened=false;}
        else{var L=ta.value.length;ta.setSelectionRange(L,L);}
      },0);
      box.querySelector('[data-es=nudge-cancel]').addEventListener('click',function(ev){
        ev.preventDefault();ev.stopPropagation();editingId=null;editingDraft='';render();
      },true);
      box.querySelector('[data-es=nudge-add]').addEventListener('click',function(ev){
        ev.preventDefault();ev.stopPropagation();saveNudge(n,ta.value);
      },true);
      ta.addEventListener('keydown',function(ev){
        ev.stopPropagation();
        if(ev.key==='Escape'){editingId=null;editingDraft='';render();}
        if(ev.key==='Enter'&&(ev.metaKey||ev.ctrlKey))saveNudge(n,ta.value);
      },true);
      card.appendChild(box);
    }else if(inFlightNudge(n)){
      var wk=document.createElement('div');wk.className='es-summary-correcting';
      wk.innerHTML='<span class="es-dot es-dot-amber" style="display:inline-block;margin-right:5px"></span>agent re-working your correction…';
      card.appendChild(wk);
    }else if(!isOpen(n)&&!isFrozen()){
      var act=document.createElement('div');act.className='es-summary-row es-thread-act';
      var nudge=document.createElement('button');nudge.className='es-nudge';nudge.type='button';
      nudge.title='Nudge the summary';nudge.textContent='✎';
      nudge.addEventListener('click',function(ev){
        ev.preventDefault();ev.stopPropagation();
        editingId=n.id;editingDraft=latestSummary(n);editingJustOpened=true;render();
      },true);
      act.appendChild(nudge);
      card.appendChild(act);
    }
    return card;
  }

  // Saving a nudge appends it to the thread and ships the whole note to
  // /__isore/dispatch immediately — before "Dispatch corrections" — so a
  // pending nudge is never only client-side state and reconcile() on reload
  // restores it from the server's full set. DispatchedAt is left untouched, so
  // the nudge stays pending (not yet the agent's work) until dispatch.
  function saveNudge(n,text){
    var v=(text||'').trim(),base=latestSummary(n);
    if(!v||v===base.trim()){editingId=null;editingDraft='';render();return;}
    n.thread=(n.thread||[]).concat([{kind:'nudge',base:base,text:v,at:Date.now()}]);
    n.editedAt=Date.now();
    editingId=null;editingDraft='';save();render();
    ship(JSON.stringify([n]));
  }

  // Dispatching corrections re-stamps DispatchedAt past each pending nudge's At,
  // flipping it from pending to in-flight so the agent is woken (server-side
  // NewSince gates on DispatchedAt). agentStatus is reset so the badge shows the
  // fresh dispatched→working→fixed lifecycle rather than the stale prior state.
  function dispatchCorrections(){
    if(isFrozen())return;
    var pending=notes.filter(hasPendingCorrection);
    if(pending.length===0)return;
    var now=Date.now();
    pending.forEach(function(n){n.dispatchedAt=now;n.agentStatus='';});
    save();render();
    ship(JSON.stringify(pending));
  }

  // ── footer / primary action button ───────────────────────────
  function buildFooter(){
    var foot=document.createElement('div');
    dispatchBtn=document.createElement('button');dispatchBtn.setAttribute('data-es','dispatch');
    var edited=countEdited(),pending=pendingNudgeCount();
    if(isFrozen()){
      dispatchBtn.textContent='Handing off…';dispatchBtn.disabled=true;dispatchBtn.className='';
    }else if(edited>0){
      dispatchBtn.innerHTML='Hand off '+edited+'<span class="es-kbd">⌘⏎</span>';
      dispatchBtn.disabled=false;dispatchBtn.className='';
    }else if(pending>0){
      dispatchBtn.innerHTML='Dispatch '+pending+' correction'+(pending===1?'':'s')+'<span class="es-kbd">⌘⏎</span>';
      dispatchBtn.disabled=false;dispatchBtn.className='';
    }else if(notes.length>0){
      dispatchBtn.textContent='Handed off ✓';dispatchBtn.disabled=true;dispatchBtn.className='es-done';
    }else{
      dispatchBtn.textContent='Hand off';dispatchBtn.disabled=true;dispatchBtn.className='';
    }
    dispatchBtn.addEventListener('click',function(ev){
      ev.preventDefault();ev.stopPropagation();
      if(edited>0)dispatch();else if(pending>0)dispatchCorrections();
    },true);
    foot.appendChild(dispatchBtn);
    return foot;
  }

  function applyCollapsed(){
    if(toolbar)toolbar.style.display=collapsed?'none':'block';
    if(!launcher)return;
    launcher.style.display=collapsed?'flex':'none';
    var pending=0;
    notes.forEach(function(n){if(isOpen(n))pending++;});
    var badge=launcher.querySelector('.es-launcher-badge');
    if(pending>0){
      if(!badge){badge=document.createElement('span');badge.className='es-launcher-badge';launcher.appendChild(badge);}
      badge.textContent=String(pending);
    }else if(badge){badge.remove();}
  }

  // ── badges (light DOM pins) ───────────────────────────────────
  // Repaint in place: recreating badge nodes every ensure() tick replays
  // the scale-in animation and the whole overlay appears to blink.
  function renderBadges(){
    var keep={};
    if(!collapsed)notes.forEach(function(n){keep[n.id]=true;});
    document.querySelectorAll('[data-es=badge]').forEach(function(b){
      if(!keep[b.getAttribute('data-es-id')])b.remove();
    });
    document.querySelectorAll('[data-es-marked]').forEach(function(el){
      var has=false,bs=badgeLayer.querySelectorAll('[data-es=badge]');
      for(var j=0;j<bs.length;j++){if(bs[j].__esEl===el&&keep[bs[j].getAttribute('data-es-id')]){has=true;break;}}
      if(!has)el.removeAttribute('data-es-marked');
    });
    if(collapsed)return;
    notes.forEach(function(n,i){
      try{
        var el=document.querySelector(n.selector);if(!el)return;
        var badge=null,curr=badgeLayer.querySelectorAll('[data-es=badge]');
        for(var j=0;j<curr.length;j++){if(curr[j].getAttribute('data-es-id')===n.id){badge=curr[j];break;}}
        if(!badge){
          badge=document.createElement('div');badge.setAttribute('data-es','badge');
          badge.setAttribute('data-es-id',n.id);
          badge.style.pointerEvents='auto';
          badge.addEventListener('mouseenter',function(ev){ev.stopPropagation();showPopover(el,badge.__esNote);},true);
          badge.addEventListener('mouseleave',function(){scheduleHidePopover();},true);
          badgeLayer.appendChild(badge);
        }
        var state=n.agentStatus==='working'?'working':(inFlightNudge(n)||(n.dispatchedAt>0&&!n.fixedAt)?'dispatched':(!isOpen(n)?'fixed':(n.dispatchedAt>0?'dispatched':'pending')));
        badge.__esNote=n;badge.__esEl=el;
        badge.textContent=state==='fixed'?'✓':String(i+1);
        badge.setAttribute('data-es-state',state);
        el.setAttribute('data-es-marked',state);
        positionBadge(badge,el);
      }catch(_){}
    });
  }

  function positionBadge(badge,el){
    var r=el.getBoundingClientRect();
    badge.style.left=(r.right-11)+'px';
    badge.style.top=(r.top-11)+'px';
  }

  function repositionBadges(){
    if(!badgeLayer)return;
    badgeLayer.querySelectorAll('[data-es=badge]').forEach(function(b){
      if(b.__esEl)positionBadge(b,b.__esEl);
    });
  }
  // Badges must track their target element through ANY layout change, not
  // just scroll/resize: the page's own JS can toggle a class or animate a
  // transition (e.g. opening a side menu) with no scroll/resize event at
  // all, so a rAF loop is the only reliable way to stay glued to it.
  (function repositionLoop(){
    repositionBadges();
    requestAnimationFrame(repositionLoop);
  })();

  // ── hover popover ──────────────────────────────────────────
  var popEl=null,popTimer=null;
  function showPopover(el,note){
    hidePopover();clearTimeout(popTimer);
    var r=el.getBoundingClientRect();
    var pop=document.createElement('div');pop.setAttribute('data-es','popover');
    var left=r.right+10;if(left+350>window.innerWidth)left=r.left-350;
    if(left<8)left=8;
    var top=r.top;if(top+180>window.innerHeight)top=window.innerHeight-190;
    if(top<8)top=8;
    pop.style.left=left+'px';pop.style.top=top+'px';
    var agentLine=note.agentStatus?('<div class="es-pop-agent">'+
      (note.agentStatus==='working'?'&#9203; agent working&hellip;':'&#10003; '+esc(latestSummary(note)||'fixed'))+'</div>'):'';
    var commentsLine=(note.comments&&note.comments.length)?
      note.comments.map(function(c){return '<div class="es-pop-comment">'+esc(c.text)+'</div>';}).join(''):'';
    pop.innerHTML='<div class="es-pop-note">'+esc(note.note)+'</div>'+commentsLine+agentLine+
      '<div class="es-pop-meta">'+esc(note.selector)+'</div>'+
      '<div class="es-pop-actions">'+
      '<button data-es="pop-edit" class="es-btn es-btn-edit">Comment</button>'+
      '<button data-es="pop-del" class="es-btn es-btn-del">Dismiss</button></div>';
    pop.addEventListener('mouseenter',function(){clearTimeout(popTimer);},true);
    pop.addEventListener('mouseleave',function(){scheduleHidePopover();},true);
    pop.querySelector('[data-es=pop-edit]').addEventListener('click',function(ev){ev.stopPropagation();hidePopover();openInlineComment(el,note);},true);
    pop.querySelector('[data-es=pop-del]').addEventListener('click',function(ev){ev.stopPropagation();confirmDelete(el,note,pop);},true);
    root.appendChild(pop);popEl=pop;
  }
  function hidePopover(){if(popEl){popEl.remove();popEl=null;}}
  function scheduleHidePopover(){popTimer=setTimeout(hidePopover,250);}

  // ── delete confirm ─────────────────────────────────────────
  function confirmDelete(el,note,pop){
    if(isFrozen())return;
    pop.querySelector('.es-pop-actions').innerHTML=
      '<button data-es="del-cancel" class="es-btn es-btn-cancel">Cancel</button>'+
      '<button data-es="del-confirm" class="es-btn es-btn-confirm">Confirm</button>';
    pop.querySelector('[data-es=del-cancel]').addEventListener('click',function(ev){ev.stopPropagation();hidePopover();},true);
    pop.querySelector('[data-es=del-confirm]').addEventListener('click',function(ev){
      ev.stopPropagation();hidePopover();
      notes=notes.filter(function(n){return n.id!==note.id;});save();render();
      shipDelete(note.id);
    },true);
  }

  // ── note card (flag) ───────────────────────────────────────
  function buildNoteCard(el){
    var r=el.getBoundingClientRect();
    var card=document.createElement('div');card.setAttribute('data-es','note');
    var left=r.left,top=r.bottom+6;
    if(left+302>window.innerWidth)left=window.innerWidth-312;
    if(left<8)left=8;
    if(top+220>window.innerHeight)top=r.top-220;
    if(top<8)top=8;
    card.style.left=left+'px';card.style.top=top+'px';
    return card;
  }

  function openInline(el){
    if(dialogOpen||isFrozen())return;dialogOpen=true;
    if(hi)hi.style.display='none';
    if(inspect)inspect.style.display='none';
    if(flagpill)flagpill.style.display='none';
    var sel=selectorOf(el),r=el.getBoundingClientRect();
    var card=buildNoteCard(el);
    card.innerHTML='<div class="es-note-ctx">'+
      '<div class="es-note-thumb"></div>'+
      '<div class="es-note-sel">'+esc(sel)+'</div>'+
      '<div class="es-dim">'+Math.round(r.width)+' × '+Math.round(r.height)+'</div>'+
      '<div class="es-captured">◱ captured</div></div>'+
      '<textarea data-es="in" rows="3" placeholder="Add a note…"></textarea>'+
      '<div class="es-note-foot"><span class="es-note-esc">esc to cancel</span>'+
      '<div class="es-btnrow" style="margin-top:0">'+
      '<button data-es="cancel" class="es-ghost">Cancel</button>'+
      '<button data-es="add" class="es-accentbtn">Add<span class="es-kbd">⌘⏎</span></button></div></div>';
    root.appendChild(card);
    var input=card.querySelector('[data-es=in]');input.focus();
    var close=function(){card.remove();dialogOpen=false;render();};
    card.querySelector('[data-es=cancel]').addEventListener('click',function(ev){ev.stopPropagation();close();},true);
    card.querySelector('[data-es=add]').addEventListener('click',function(ev){
      ev.stopPropagation();var v=input.value.trim();
      if(v){
        var now=Date.now();
        var note={id:id(),selector:sel,label:((el.innerText||el.textContent||'').trim()).slice(0,80),
          note:v,url:location.href,createdAt:now,editedAt:now,dispatchedAt:0,
          dim:Math.round(r.width)+' × '+Math.round(r.height)};
        notes.push(note);save();
      }close();
    },true);
    input.addEventListener('keydown',function(ev){
      ev.stopPropagation();
      if(ev.key==='Escape')close();
      if(ev.key==='Enter'&&(ev.metaKey||ev.ctrlKey))card.querySelector('[data-es=add]').click();
    },true);
  }

  // ── inline edit (comment) ───────────────────────────────────
  function openInlineComment(el,note){
    if(dialogOpen||isFrozen())return;dialogOpen=true;
    var card=buildNoteCard(el);
    card.innerHTML='<div class="es-note-esc" style="margin-bottom:8px">Add a comment</div>'+
      '<textarea data-es="in" rows="3" placeholder="Add a comment…"></textarea>'+
      '<div class="es-note-foot"><span class="es-note-esc">esc to cancel</span>'+
      '<div class="es-btnrow" style="margin-top:0">'+
      '<button data-es="cancel" class="es-ghost">Cancel</button>'+
      '<button data-es="save" class="es-accentbtn">Save<span class="es-kbd">⌘⏎</span></button></div></div>';
    root.appendChild(card);
    var input=card.querySelector('[data-es=in]');input.focus();
    var close=function(){card.remove();dialogOpen=false;render();};
    card.querySelector('[data-es=cancel]').addEventListener('click',function(ev){ev.stopPropagation();close();},true);
    card.querySelector('[data-es=save]').addEventListener('click',function(ev){
      ev.stopPropagation();var v=input.value.trim();
      if(v){
        var now=Date.now();
        for(var i=0;i<notes.length;i++){
          if(notes[i].id===note.id){
            notes[i].comments=(notes[i].comments||[]).concat([{text:v,at:now}]);
            notes[i].editedAt=now;
            break;
          }
        }save();
        shipComment(note.id,v);
      }close();
    },true);
    input.addEventListener('keydown',function(ev){
      ev.stopPropagation();
      if(ev.key==='Escape')close();
      if(ev.key==='Enter'&&(ev.metaKey||ev.ctrlKey))card.querySelector('[data-es=save]').click();
    },true);
  }

  // ── dispatch (hand off the batch) ─────────────────────────────
  function dispatch(){
    if(collapsed||notes.length===0||isFrozen())return;
    var edited=notes.filter(function(n){return n.dispatchedAt===0||n.editedAt>n.dispatchedAt;});
    if(edited.length===0)return;
    var now=Date.now();
    notes.forEach(function(n){if(n.dispatchedAt===0||n.editedAt>n.dispatchedAt){n.dispatchedAt=now;n.fixedAt=0;n.agentStatus='';}});
    save();render();
    ship(JSON.stringify(edited));
  }

  // ── hover highlight / inspect label / flag pill ─────────────
  document.addEventListener('mousemove',function(e){
    if(!hi||collapsed)return;var el=e.target;
    if(dialogOpen||isES(el)||isFrozen()){
      hi.style.display='none';if(inspect)inspect.style.display='none';if(flagpill)flagpill.style.display='none';return;
    }
    var r=el.getBoundingClientRect();
    Object.assign(hi.style,{display:'block',left:r.left+'px',top:r.top+'px',width:r.width+'px',height:r.height+'px'});
    if(inspect){
      inspect.innerHTML='<span class="es-in-sel">'+esc(selectorOf(el))+'</span>'+
        '<span class="es-in-dim">'+Math.round(r.width)+' × '+Math.round(r.height)+'</span>';
      var il=r.left,it=r.bottom+4;
      Object.assign(inspect.style,{display:'block',left:il+'px',top:it+'px'});
    }
    if(flagpill){
      Object.assign(flagpill.style,{display:'flex',left:(r.left)+'px',top:(r.top-26)+'px'});
    }
  },true);

  document.addEventListener('click',function(e){
    if(collapsed||isFrozen())return;var el=e.target;
    if(isES(el))return;
    e.preventDefault();e.stopPropagation();
    openInline(el);
  },true);

  // ── keyboard: ⌥I wake/sleep, ⌘⏎ hand off / dispatch corrections ──
  document.addEventListener('keydown',function(ev){
    if(ev.altKey&&(ev.key==='i'||ev.key==='I')){
      ev.preventDefault();if(collapsed)wake();else sleep();return;
    }
    if((ev.metaKey||ev.ctrlKey)&&ev.key==='Enter'){
      // Note/nudge editors own ⌘⏎ locally (their own keydown handler,
      // which stops propagation) — this only fires for the queue-level action.
      if(dialogOpen||editingId||collapsed)return;
      if(countEdited()>0)dispatch();else if(pendingNudgeCount()>0)dispatchCorrections();
    }
  },true);

  // ── init ───────────────────────────────────────────────────
  function init(){load();ensure();listen();}
  window.__es={ensure:ensure,render:render,notes:function(){return notes;},enabled:function(){return !collapsed;},frozen:isFrozen};
  init();
  setInterval(ensure,1000);
  // Fallback while the SSE stream is down (e.g. proxy briefly gone): keep
  // polling so a frozen overlay can still thaw if reconnects keep failing.
  setInterval(function(){if(proxyMode&&!sseLive)reconcile();},5000);
})();
true
`
