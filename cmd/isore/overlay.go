package main

import "encoding/json"

type deleteEvent struct {
	ID string `json:"id"`
}

type editEvent struct {
	ID   string `json:"id"`
	Note string `json:"note"`
}

func parseDeleteEvent(data []byte) (deleteEvent, bool) {
	var e deleteEvent
	if err := json.Unmarshal(data, &e); err != nil || e.ID == "" {
		return e, false
	}
	return e, true
}

func parseEditEvent(data []byte) (editEvent, bool) {
	var e editEvent
	if err := json.Unmarshal(data, &e); err != nil || e.ID == "" {
		return e, false
	}
	return e, true
}

const overlayJS = `
(function(){
  if(window.__es){window.__es.ensure();return;}
  var notes=[],enabled=false,dialogOpen=false;

  function isES(el){return !!(el&&el.closest&&(el.closest('[data-es]')||el.closest('[data-es-root]')));}

  var proxyMode=!window.esDispatch;
  function ship(json){
    if(window.esDispatch){window.esDispatch(json);return;}
    fetch('/__isore/dispatch',{method:'POST',headers:{'Content-Type':'application/json'},body:json});
  }
  function applyServerNotes(list){
    var touched=false;
    list.forEach(function(c){
      var found=false;
      for(var i=0;i<notes.length;i++){
        if(notes[i].id===c.id){
          notes[i].agentStatus=c.agentStatus;
          notes[i].agentSummary=c.agentSummary;
          notes[i].fixedAt=c.fixedAt;
          found=true;touched=true;
        }
      }
      if(!found&&c.url===location.href){notes.push(c);touched=true;}
    });
    if(touched){save();applyEnabled();}
  }
  // reconcile pulls the authoritative note state: SSE only streams diffs, so
  // any gap in the stream (tab closed, proxy restarted) is healed here.
  function reconcile(){
    fetch('/__isore/notes').then(function(r){return r.json();})
      .then(applyServerNotes).catch(function(){});
  }
  function listen(){
    if(!proxyMode||window.__esSSE)return;
    try{
      var es=new EventSource('/__isore/events');window.__esSSE=es;
      // onopen refires on every auto-reconnect — reconcile each time, since
      // whatever streamed while the connection was down is gone for good.
      es.onopen=function(){sseLive=true;reconcile();renderStatus();};
      es.onerror=function(){sseLive=false;renderStatus();};
      es.addEventListener('reload',function(){location.reload();});
      es.addEventListener('notes',function(ev){
        try{applyServerNotes(JSON.parse(ev.data)||[]);}catch(_){}
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
        if(same.length>1)s+=':nth-of-type('+(Array.prototype.indexOf.call(p.children,cur)+1)+')';}
      parts.unshift(s);cur=cur.parentNode;depth++;
    }
    return parts.join(' > ')||el.tagName.toLowerCase();
  }

  // ── stylesheets ────────────────────────────────────────────
  // Chrome lives in a shadow root so page CSS (e.g. global button rules)
  // cannot restyle it; badges and outlines attach to page elements, so their
  // rules must stay in the light DOM.
  function injectShadowStyles(){
    if(root.querySelector('#es-css'))return;
    var st=document.createElement('style');st.id='es-css';
    st.textContent=[
      ':host{all:initial}',
      '*{box-sizing:border-box}',
      '@keyframes es-fadeIn{from{opacity:0;transform:translateY(4px)}to{opacity:1;transform:translateY(0)}}',
      '@keyframes es-pulse{0%,100%{box-shadow:0 0 0 0 rgba(255,59,48,0)}50%{box-shadow:0 0 0 6px rgba(255,59,48,0.15)}}',
      '',
      '[data-es=hi]{position:fixed;pointer-events:none;z-index:2147483646;',
        'border:2px solid rgba(255,59,48,0.6);background:rgba(255,59,48,0.06);',
        'display:none;border-radius:4px;',
        'box-shadow:0 0 8px rgba(255,59,48,0.15);',
        'transition:all .12s ease-out}',
      '',
      '[data-es=launcher]{position:fixed;right:16px;bottom:16px;z-index:2147483647;',
        'width:44px;height:44px;border-radius:14px;cursor:pointer;border:none;padding:0;',
        'background:linear-gradient(135deg,#7c6bfb,#5a3ff0);',
        'box-shadow:0 4px 16px rgba(90,63,240,.4),0 0 0 1px rgba(255,255,255,0.08);',
        'display:flex;align-items:center;justify-content:center;',
        'font:700 15px/1 system-ui,-apple-system,sans-serif;color:#fff;',
        'transition:transform .15s ease,box-shadow .15s ease;',
        'animation:es-fadeIn .15s ease-out}',
      '[data-es=launcher]:hover{transform:scale(1.06);box-shadow:0 6px 20px rgba(90,63,240,.5),0 0 0 1px rgba(255,255,255,0.1)}',
      '[data-es=launcher] .es-launcher-badge{position:absolute;top:-4px;right:-4px;',
        'min-width:16px;height:16px;border-radius:999px;background:#ff3b30;color:#fff;',
        'font:700 9.5px/16px system-ui,-apple-system,sans-serif;text-align:center;padding:0 3px;',
        'box-shadow:0 0 0 2px #1e1e1e}',
      '[data-es=toolbar]{position:fixed;right:16px;bottom:16px;z-index:2147483647;',
        'background:#1e1e1e;border-radius:14px;',
        'box-shadow:0 4px 20px rgba(0,0,0,.45),0 0 0 1px rgba(255,255,255,0.06);',
        'backdrop-filter:blur(8px);padding:12px 14px;',
        'width:fit-content;max-width:250px;',
        'font:12px/1.4 system-ui,-apple-system,sans-serif;',
        'animation:es-fadeIn .15s ease-out}',
      '.es-toolbar-row{display:flex;align-items:center;gap:8px}',
      '.es-brand{display:flex;align-items:center;gap:7px;flex:1;min-width:0}',
      '.es-brand-mark{flex-shrink:0;width:20px;height:20px;border-radius:6px;',
        'background:linear-gradient(135deg,#7c6bfb,#5a3ff0);',
        'display:flex;align-items:center;justify-content:center;',
        'font:700 11px/1 system-ui,-apple-system,sans-serif;color:#fff}',
      '.es-wordmark{font:600 13px/1 system-ui,-apple-system,sans-serif;color:#e8e8e8;',
        'letter-spacing:.2px;white-space:nowrap}',
      '.es-minimize{flex-shrink:0;background:none;border:none;cursor:pointer;',
        'color:#666;font:14px/1 system-ui,-apple-system,sans-serif;padding:2px 4px;border-radius:4px}',
      '.es-minimize:hover{color:#ccc;background:rgba(255,255,255,0.06)}',
      '.es-toolbar-desc{font:11px/1.35 system-ui,-apple-system,sans-serif;',
        'color:#777;margin:6px 0 4px}',
      '.es-toolbar-status{font:11px/1.35 system-ui,-apple-system,sans-serif;',
        'color:#666;margin:0 0 10px;min-height:14px}',
      '.es-toolbar-status .es-dot{display:inline-block;width:7px;height:7px;',
        'border-radius:50%;margin-right:5px;background:#555;vertical-align:middle}',
      '.es-toolbar-status.es-live .es-dot{background:#1a7f4b}',
      '.es-toolbar-status.es-busy .es-dot{background:#f5a623;',
        'animation:es-pulse 1.2s ease-in-out infinite}',
      '[data-es=panel]{margin:0 0 10px;max-height:220px;overflow-y:auto;',
        'border-top:1px solid rgba(255,255,255,0.06);padding-top:8px;display:none}',
      '[data-es=panel].es-open{display:block}',
      '.es-item{display:flex;gap:7px;align-items:flex-start;',
        'padding:5px 4px;border-radius:6px;cursor:pointer;',
        'font:12px/1.4 system-ui,-apple-system,sans-serif;color:#ccc}',
      '.es-item:hover{background:rgba(255,255,255,0.05)}',
      '.es-item .es-item-dot{flex-shrink:0;width:8px;height:8px;',
        'border-radius:50%;margin-top:4px;background:#ff3b30}',
      '.es-item.es-working .es-item-dot{background:#f5a623}',
      '.es-item.es-fixed .es-item-dot{background:#1a7f4b}',
      '.es-item .es-item-body{min-width:0}',
      '.es-item .es-item-note{white-space:nowrap;overflow:hidden;',
        'text-overflow:ellipsis;max-width:170px}',
      '.es-item.es-fixed .es-item-note{color:#777;text-decoration:line-through}',
      '.es-item .es-item-summary{font-size:11px;color:#7fd6a4;',
        'white-space:normal;margin-top:1px}',
      '.es-panel-toggle{background:none;border:none;color:#888;',
        'font:11px system-ui,-apple-system,sans-serif;cursor:pointer;padding:0;margin:0 0 8px;',
        'text-align:left;width:auto;display:block}',
      '.es-panel-toggle:hover{color:#ccc}',
      '',
      '.es-switch{position:relative;width:36px;height:20px;cursor:pointer;flex-shrink:0}',
      '.es-switch input{opacity:0;width:0;height:0}',
      '.es-switch .es-slider{position:absolute;inset:0;',
        'background:#333;border-radius:20px;transition:all .2s ease}',
      '.es-switch .es-slider:before{content:"";position:absolute;',
        'width:16px;height:16px;border-radius:50%;left:2px;top:2px;',
        'background:#888;transition:all .2s ease}',
      '.es-switch input:checked+.es-slider{background:#5a3ff0}',
      '.es-switch input:checked+.es-slider:before{transform:translateX(16px);background:#fff}',
      '',
      '.es-toolbar-foot{margin-top:2px}',
      '[data-es=dispatch]{border:none;border-radius:7px;cursor:pointer;width:100%;',
        'font:600 12px/1 system-ui,-apple-system,sans-serif;',
        'color:#fff;background:#5a3ff0;',
        'padding:8px 0;white-space:nowrap;',
        'transition:all .2s ease}',
      '[data-es=dispatch]:not(:disabled):hover{filter:brightness(1.15)}',
      '[data-es=dispatch]:not(:disabled):active{transform:scale(0.97)}',
      '[data-es=dispatch]:disabled{background:#2a2a2a;color:#555;cursor:default}',
      '[data-es=dispatch].es-green{background:#1a7f4b}',
      '',
      '[data-es=popover]{position:fixed;z-index:2147483645;',
        'background:#1e1e1e;color:#e0e0e0;',
        'padding:12px 16px;border-radius:10px;max-width:340px;min-width:220px;',
        'font:13px/1.5 system-ui,-apple-system,sans-serif;',
        'box-shadow:0 12px 40px rgba(0,0,0,.5),0 0 0 1px rgba(255,255,255,0.06);',
        'backdrop-filter:blur(8px);',
        'animation:es-fadeIn .15s ease-out}',
      '.es-pop-note{margin-bottom:8px;white-space:pre-wrap;word-break:break-word;',
        'color:#e8e8e8}',
      '.es-pop-meta{font-size:11px;color:#666;margin-bottom:8px}',
      '.es-pop-agent{font-size:12px;color:#7fd6a4;margin-bottom:8px}',
      '.es-pop-actions{display:flex;gap:6px;justify-content:flex-end}',
      '',
      '.es-btn{padding:5px 12px;font-size:12px;border-radius:5px;',
        'cursor:pointer;border:none;font:600 12px/1 system-ui,-apple-system,sans-serif;',
        'transition:all .15s ease}',
      '.es-btn:hover{filter:brightness(1.15)}',
      '.es-btn-edit{background:#2a2a2a;color:#bbb;border:1px solid #444}',
      '.es-btn-del{background:#3a1a1a;color:#e57373;border:1px solid #5a2a2a}',
      '.es-btn-cancel{background:#2a2a2a;color:#bbb;border:1px solid #444}',
      '.es-btn-confirm{background:#c62828;color:#fff}',
      '.es-btn-save{background:#1a7f4b;color:#fff}',
      '',
      '[data-es=inline]{position:fixed;z-index:2147483647;',
        'background:#1e1e1e;color:#e0e0e0;',
        'padding:14px 16px;border-radius:10px;width:290px;',
        'font:13px/1.4 system-ui,-apple-system,sans-serif;',
        'box-shadow:0 12px 40px rgba(0,0,0,.5),0 0 0 1px rgba(255,255,255,0.06);',
        'backdrop-filter:blur(8px);',
        'animation:es-fadeIn .15s ease-out}',
      '.es-inline-hint{font-size:11px;color:#666;margin-bottom:8px;',
        'white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
      '[data-es=inline] textarea{width:100%;box-sizing:border-box;',
        'font:13px/1.5 system-ui,-apple-system,sans-serif;',
        'padding:8px 10px;background:#141414;color:#e8e8e8;',
        'border:1px solid #333;border-radius:6px;resize:vertical;',
        'outline:none;transition:border-color .15s ease}',
      '[data-es=inline] textarea:focus{border-color:#7c6bfb}',
      '[data-es=inline] textarea::placeholder{color:#555}',
      '.es-inline-actions{display:flex;gap:6px;justify-content:flex-end;margin-top:10px}',
      '.es-inline-shortcut{font-size:10px;color:#555;margin-top:6px;text-align:right}'
    ].join('');
    root.appendChild(st);
  }

  function injectLightStyles(){
    if(document.getElementById('es-css-light'))return;
    var st=document.createElement('style');st.id='es-css-light';
    st.textContent=[
      '@keyframes es-scaleIn{from{opacity:0;transform:scale(0.5)}to{opacity:1;transform:scale(1)}}',
      '@keyframes es-flash{0%,100%{outline:2px solid transparent}',
        '50%{outline:3px solid rgba(255,59,48,0.8)}}',
      '[data-es=badge]{position:fixed;',
        'min-width:20px;height:20px;border-radius:999px;padding:0 5px;',
        'background:#e5484d;color:#fff;',
        'display:flex;align-items:center;justify-content:center;',
        'font:650 10.5px/1 system-ui,-apple-system,sans-serif;',
        'cursor:pointer;z-index:2147483640;',
        'box-shadow:0 1px 2px rgba(0,0,0,.25),0 4px 12px -2px rgba(0,0,0,.3);',
        'border:1.5px solid rgba(255,255,255,.9);',
        'animation:es-scaleIn .2s ease-out;',
        'transition:transform .15s ease,box-shadow .15s ease;user-select:none}',
      '[data-es=badge]:hover{transform:scale(1.15)}',
      '[data-es=badge][data-es-state=working]{background:#f5a623}',
      '[data-es=badge][data-es-state=fixed]{background:#2f9e68}',
      '[data-es-marked]{outline:1.5px solid rgba(229,72,77,.35)!important;',
        'outline-offset:3px;border-radius:3px;',
        'transition:outline-color .18s ease,outline-offset .18s ease}',
      '[data-es-marked=working]{outline-color:rgba(245,166,35,.4)!important}',
      '[data-es-marked=fixed]{outline-color:rgba(47,158,104,.4)!important}',
      '[data-es-marked]:hover{outline-width:2.5px;outline-offset:2px;',
        'outline-color:rgba(229,72,77,.9)!important}',
      '[data-es-marked=working]:hover{outline-color:rgba(245,166,35,.95)!important}',
      '[data-es-marked=fixed]:hover{outline-color:rgba(47,158,104,.95)!important}'
    ].join('');
    document.head.appendChild(st);
  }

  // ── persistence ────────────────────────────────────────────
  function storageKey(){return 'es_notes_'+location.href;}
  function save(){
    try{localStorage.setItem(storageKey(),JSON.stringify(notes));
        localStorage.setItem('es_enabled',enabled?'1':'0');
        localStorage.setItem('es_collapsed',collapsed?'1':'0');}catch(_){}
  }
  function load(){
    try{var s=localStorage.getItem(storageKey());if(s)notes=JSON.parse(s)||[];
        var e=localStorage.getItem('es_enabled');enabled=e==='1';
        var c=localStorage.getItem('es_collapsed');collapsed=c===null?true:c==='1';}catch(_){notes=[];}
  }

  // ── DOM scaffold ───────────────────────────────────────────
  var host=null,root=null,hi=null,toolbar=null,toggleInput=null,dispatchBtn=null;
  var badgeLayer=null,launcher=null,collapsed=true;

  function ensure(){
    if(!document.body)return;
    if(!host||!document.body.contains(host)){
      host=document.createElement('div');host.setAttribute('data-es-root','');
      root=host.attachShadow({mode:'open'});
      document.body.appendChild(host);
    }
    if(!badgeLayer||!document.body.contains(badgeLayer)){
      badgeLayer=document.createElement('div');badgeLayer.setAttribute('data-es','badge-layer');
      badgeLayer.style.cssText='position:fixed;top:0;left:0;width:0;height:0;overflow:visible;z-index:2147483640;pointer-events:none';
      document.body.appendChild(badgeLayer);
    }
    injectShadowStyles();
    injectLightStyles();
    if(!hi||!root.contains(hi)){
      hi=document.createElement('div');hi.setAttribute('data-es','hi');
      root.appendChild(hi);
    }
    if(!launcher||!root.contains(launcher)){
      launcher=document.createElement('button');launcher.setAttribute('data-es','launcher');
      launcher.type='button';launcher.textContent='is';launcher.title='isore';
      launcher.addEventListener('click',function(ev){
        ev.preventDefault();ev.stopPropagation();collapsed=false;save();applyCollapsed();
      },true);
      root.appendChild(launcher);
    }
    if(!toolbar||!root.contains(toolbar)){
      toolbar=document.createElement('div');toolbar.setAttribute('data-es','toolbar');

      var row=document.createElement('div');row.className='es-toolbar-row';
      var brand=document.createElement('div');brand.className='es-brand';
      var mark=document.createElement('span');mark.className='es-brand-mark';mark.textContent='is';
      var word=document.createElement('span');word.className='es-wordmark';word.textContent='isore';
      brand.appendChild(mark);brand.appendChild(word);
      var sw=document.createElement('label');sw.className='es-switch';
      toggleInput=document.createElement('input');toggleInput.type='checkbox';
      toggleInput.addEventListener('change',function(ev){ev.stopPropagation();toggle();},true);
      var slider=document.createElement('span');slider.className='es-slider';
      sw.appendChild(toggleInput);sw.appendChild(slider);
      var minBtn=document.createElement('button');minBtn.className='es-minimize';minBtn.type='button';
      minBtn.textContent='–';minBtn.title='Minimize';
      minBtn.addEventListener('click',function(ev){
        ev.preventDefault();ev.stopPropagation();collapsed=true;save();applyCollapsed();
      },true);
      row.appendChild(brand);row.appendChild(sw);row.appendChild(minBtn);

      var desc=document.createElement('div');desc.className='es-toolbar-desc';
      desc.innerHTML='Click elements to annotate.';

      var status=document.createElement('div');status.className='es-toolbar-status';
      status.setAttribute('data-es','status');

      var panelToggle=document.createElement('button');panelToggle.className='es-panel-toggle';
      panelToggle.setAttribute('data-es','panel-toggle');
      panelToggle.textContent='\u25b8 notes';
      panelToggle.addEventListener('click',function(ev){
        ev.preventDefault();ev.stopPropagation();
        var pn=root.querySelector('[data-es=panel]');if(!pn)return;
        var open=pn.className.indexOf('es-open')<0;
        pn.className=open?'es-open':'';pn.setAttribute('data-es','panel');
        panelToggle.textContent=(open?'\u25be':'\u25b8')+' notes';
        if(open)renderPanel();
      },true);

      var panel=document.createElement('div');panel.setAttribute('data-es','panel');

      var foot=document.createElement('div');foot.className='es-toolbar-foot';
      dispatchBtn=document.createElement('button');dispatchBtn.setAttribute('data-es','dispatch');
      dispatchBtn.disabled=true;
      dispatchBtn.addEventListener('click',function(ev){ev.preventDefault();ev.stopPropagation();dispatch();},true);
      foot.appendChild(dispatchBtn);

      toolbar.appendChild(row);toolbar.appendChild(desc);toolbar.appendChild(status);
      toolbar.appendChild(panelToggle);toolbar.appendChild(panel);toolbar.appendChild(foot);
      root.appendChild(toolbar);
    }
    applyEnabled();
  }

  // ── agent status line ──────────────────────────────────────
  var sseLive=false;
  function renderStatus(){
    var st=root?root.querySelector('[data-es=status]'):null;if(!st)return;
    var working=0,fixed=0;
    notes.forEach(function(n){
      if(n.agentStatus==='working')working++;
      if(n.agentStatus==='fixed')fixed++;
    });
    if(working>0){st.className='es-toolbar-status es-busy';
      st.innerHTML='<span class="es-dot"></span>agent working ('+working+')';}
    else if(fixed>0&&fixed===notes.length&&notes.length>0){st.className='es-toolbar-status es-live';
      st.innerHTML='<span class="es-dot"></span>all '+fixed+' fixed';}
    else if(sseLive){st.className='es-toolbar-status es-live';
      st.innerHTML='<span class="es-dot"></span>listening'+(fixed>0?' \u00b7 '+fixed+' fixed':'');}
    else if(proxyMode){st.className='es-toolbar-status';
      st.innerHTML='<span class="es-dot"></span>connecting\u2026';}
    else{st.className='es-toolbar-status';
      st.innerHTML='<span class="es-dot"></span>screenshots on dispatch';}
  }

  // ── notes panel ────────────────────────────────────────────
  function renderPanel(){
    var pn=root?root.querySelector('[data-es=panel]'):null;if(!pn)return;
    var tg=root.querySelector('[data-es=panel-toggle]');
    if(tg)tg.textContent=(pn.className.indexOf('es-open')>=0?'\u25be':'\u25b8')+' notes ('+notes.length+')';
    if(pn.className.indexOf('es-open')<0)return;
    pn.innerHTML='';
    if(notes.length===0){
      pn.innerHTML='<div class="es-item"><div class="es-item-body es-item-note" style="color:#666">no annotations yet</div></div>';
      return;
    }
    notes.forEach(function(n,i){
      var it=document.createElement('div');
      it.className='es-item'+(n.agentStatus==='working'?' es-working':(n.fixedAt?' es-fixed':''));
      var dot=document.createElement('div');dot.className='es-item-dot';
      var body=document.createElement('div');body.className='es-item-body';
      var txt=document.createElement('div');txt.className='es-item-note';
      txt.textContent=(i+1)+'. '+n.note;txt.title=n.note+'\n'+n.selector;
      body.appendChild(txt);
      if(n.agentSummary){
        var sm=document.createElement('div');sm.className='es-item-summary';
        sm.textContent='\u2713 '+n.agentSummary;body.appendChild(sm);
      }
      it.appendChild(dot);it.appendChild(body);
      it.addEventListener('click',function(ev){
        ev.stopPropagation();
        try{
          var el=document.querySelector(n.selector);if(!el)return;
          el.scrollIntoView({behavior:'smooth',block:'center'});
          el.style.animation='es-flash 0.6s ease 3';
          setTimeout(function(){el.style.animation='';},2000);
        }catch(_){}
      },true);
      pn.appendChild(it);
    });
  }

  // ── toggle ─────────────────────────────────────────────────
  function toggle(){
    enabled=!enabled;save();applyEnabled();
    if(window.esToggle)window.esToggle(enabled?'on':'off');
  }
  function anyWorking(){
    for(var i=0;i<notes.length;i++){if(notes[i].agentStatus==='working')return true;}
    return false;
  }
  function applyEnabled(){
    // The whole annotator freezes while the agent works: mid-fix edits,
    // deletes, or dispatches would collide with the agent's changes.
    if(toggleInput){toggleInput.checked=enabled;toggleInput.disabled=anyWorking();}
    if(dispatchBtn){
      var c=countEdited();
      if(anyWorking()){
        dispatchBtn.textContent='agent working\u2026';
        dispatchBtn.disabled=true;
        dispatchBtn.className='';
      }else{
        dispatchBtn.textContent=enabled?('Dispatch ('+c+')'):'Dispatch';
        dispatchBtn.disabled=!enabled||c===0;
        dispatchBtn.className=c>0&&enabled?'es-green':'';
      }
    }
    if(hi)hi.style.display='none';
    renderBadges();renderStatus();renderPanel();applyCollapsed();
  }

  function applyCollapsed(){
    if(toolbar)toolbar.style.display=collapsed?'none':'block';
    if(!launcher)return;
    launcher.style.display=collapsed?'flex':'none';
    var pending=0;
    notes.forEach(function(n){if(!n.fixedAt)pending++;});
    var badge=launcher.querySelector('.es-launcher-badge');
    if(pending>0){
      if(!badge){badge=document.createElement('span');badge.className='es-launcher-badge';launcher.appendChild(badge);}
      badge.textContent=String(pending);
    }else if(badge){badge.remove();}
  }

  // ── badges ─────────────────────────────────────────────────
  // Repaint in place: recreating badge nodes every ensure() tick replays
  // the scale-in animation and the whole overlay appears to blink.
  function renderBadges(){
    var keep={};
    if(enabled)notes.forEach(function(n){keep[n.id]=true;});
    document.querySelectorAll('[data-es=badge]').forEach(function(b){
      if(!keep[b.getAttribute('data-es-id')])b.remove();
    });
    document.querySelectorAll('[data-es-marked]').forEach(function(el){
      var has=false,bs=badgeLayer.querySelectorAll('[data-es=badge]');
      for(var j=0;j<bs.length;j++){if(bs[j].__esEl===el&&keep[bs[j].getAttribute('data-es-id')]){has=true;break;}}
      if(!has)el.removeAttribute('data-es-marked');
    });
    if(!enabled)return;
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
        var state=n.agentStatus==='working'?'working':(n.fixedAt?'fixed':'pending');
        badge.__esNote=n;badge.__esEl=el;
        badge.textContent=String(i+1);
        badge.setAttribute('data-es-state',state);
        el.setAttribute('data-es-marked',state);
        positionBadge(badge,el);
      }catch(_){}
    });
  }

  function positionBadge(badge,el){
    var r=el.getBoundingClientRect();
    badge.style.left=(r.right-9)+'px';
    badge.style.top=(r.top-9)+'px';
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
      (note.agentStatus==='working'?'&#9203; agent working&hellip;':'&#10003; '+esc(note.agentSummary||'fixed'))+'</div>'):'';
    pop.innerHTML='<div class="es-pop-note">'+esc(note.note)+'</div>'+agentLine+
      '<div class="es-pop-meta">'+esc(note.selector)+'</div>'+
      '<div class="es-pop-actions">'+
      '<button data-es="pop-edit" class="es-btn es-btn-edit">Edit</button>'+
      '<button data-es="pop-del" class="es-btn es-btn-del">Close</button></div>';
    pop.addEventListener('mouseenter',function(){clearTimeout(popTimer);},true);
    pop.addEventListener('mouseleave',function(){scheduleHidePopover();},true);
    pop.querySelector('[data-es=pop-edit]').addEventListener('click',function(ev){ev.stopPropagation();hidePopover();openInlineEdit(el,note);},true);
    pop.querySelector('[data-es=pop-del]').addEventListener('click',function(ev){ev.stopPropagation();confirmDelete(el,note,pop);},true);
    root.appendChild(pop);popEl=pop;
  }
  function hidePopover(){if(popEl){popEl.remove();popEl=null;}}
  function scheduleHidePopover(){popTimer=setTimeout(hidePopover,250);}

  // ── delete confirm ─────────────────────────────────────────
  function confirmDelete(el,note,pop){
    if(anyWorking())return;
    pop.querySelector('.es-pop-actions').innerHTML=
      '<button data-es="del-cancel" class="es-btn es-btn-cancel">Cancel</button>'+
      '<button data-es="del-confirm" class="es-btn es-btn-confirm">Confirm</button>';
    pop.querySelector('[data-es=del-cancel]').addEventListener('click',function(ev){ev.stopPropagation();hidePopover();},true);
    pop.querySelector('[data-es=del-confirm]').addEventListener('click',function(ev){
      ev.stopPropagation();hidePopover();
      notes=notes.filter(function(n){return n.id!==note.id;});save();renderBadges();
      if(window.esDelete)window.esDelete(JSON.stringify({id:note.id}));
    },true);
  }

  // ── inline card builder ────────────────────────────────────
  function buildInlineCard(el,hintText){
    var r=el.getBoundingClientRect();
    var card=document.createElement('div');card.setAttribute('data-es','inline');
    var left=r.left;var top=r.bottom+6;
    if(left+300>window.innerWidth)left=window.innerWidth-310;
    if(left<8)left=8;
    if(top+200>window.innerHeight)top=r.top-200;
    if(top<8)top=8;
    card.style.left=left+'px';card.style.top=top+'px';
    return card;
  }

  // ── inline input (new note) ────────────────────────────────
  function openInline(el){
    if(dialogOpen||anyWorking())return;dialogOpen=true;
    if(hi)hi.style.display='none';
    var card=buildInlineCard(el,selectorOf(el));
    card.innerHTML='<div class="es-inline-hint">'+esc(selectorOf(el))+'</div>'+
      '<textarea data-es="in" rows="3" placeholder="Add a note\u2026"></textarea>'+
      '<div class="es-inline-actions">'+
      '<button data-es="cancel" class="es-btn es-btn-cancel">Cancel</button>'+
      '<button data-es="add" class="es-btn es-btn-save">Add</button></div>'+
      '<div class="es-inline-shortcut">\u2318\u21E7\u23CE to save \u00b7 Esc to cancel</div>';
    root.appendChild(card);
    var input=card.querySelector('[data-es=in]');input.focus();
    var close=function(){card.remove();dialogOpen=false;};
    card.querySelector('[data-es=cancel]').addEventListener('click',function(ev){ev.stopPropagation();close();},true);
    card.querySelector('[data-es=add]').addEventListener('click',function(ev){
      ev.stopPropagation();var v=input.value.trim();
      if(v){
        var now=Date.now();
        var note={id:id(),selector:selectorOf(el),label:((el.innerText||el.textContent||'').trim()).slice(0,80),
          note:v,url:location.href,createdAt:now,editedAt:now,dispatchedAt:0};
        notes.push(note);save();renderBadges();
      }close();
    },true);
    input.addEventListener('keydown',function(ev){
      if(ev.key==='Escape')close();
      if(ev.key==='Enter'&&(ev.metaKey||ev.ctrlKey))card.querySelector('[data-es=add]').click();
    },true);
  }

  // ── inline edit ────────────────────────────────────────────
  function openInlineEdit(el,note){
    if(dialogOpen||anyWorking())return;dialogOpen=true;
    var card=buildInlineCard(el);
    card.innerHTML='<div class="es-inline-hint">Edit note</div>'+
      '<textarea data-es="in" rows="3" placeholder="Note text\u2026">'+esc(note.note)+'</textarea>'+
      '<div class="es-inline-actions">'+
      '<button data-es="cancel" class="es-btn es-btn-cancel">Cancel</button>'+
      '<button data-es="save" class="es-btn es-btn-save">Save</button></div>'+
      '<div class="es-inline-shortcut">\u2318\u21E7\u23CE to save \u00b7 Esc to cancel</div>';
    root.appendChild(card);
    var input=card.querySelector('[data-es=in]');input.focus();input.select();
    var close=function(){card.remove();dialogOpen=false;};
    card.querySelector('[data-es=cancel]').addEventListener('click',function(ev){ev.stopPropagation();close();},true);
    card.querySelector('[data-es=save]').addEventListener('click',function(ev){
      ev.stopPropagation();var v=input.value.trim();
      if(v){
        var now=Date.now();
        for(var i=0;i<notes.length;i++){
          if(notes[i].id===note.id){notes[i].note=v;notes[i].editedAt=now;break;}
        }save();renderBadges();
        if(window.esEdit)window.esEdit(JSON.stringify({id:note.id,note:v}));
      }close();
    },true);
    input.addEventListener('keydown',function(ev){
      if(ev.key==='Escape')close();
      if(ev.key==='Enter'&&(ev.metaKey||ev.ctrlKey))card.querySelector('[data-es=save]').click();
    },true);
  }

  // ── dispatch ───────────────────────────────────────────────
  function dispatch(){
    if(!enabled||notes.length===0||anyWorking())return;
    var edited=notes.filter(function(n){return n.dispatchedAt===0||n.editedAt>n.dispatchedAt;});
    if(edited.length===0){
      if(dispatchBtn){dispatchBtn.textContent='Nothing new';
        dispatchBtn.className='es-green';
        setTimeout(function(){applyEnabled();},1200);}
      return;
    }
    var now=Date.now();
    notes.forEach(function(n){if(n.dispatchedAt===0||n.editedAt>n.dispatchedAt)n.dispatchedAt=now;});
    save();renderBadges();
    ship(JSON.stringify(edited));
    if(dispatchBtn){dispatchBtn.textContent='Dispatched '+edited.length+' \u2713';
      dispatchBtn.className='es-green';
      setTimeout(function(){applyEnabled();},1400);}
  }
  function countEdited(){return notes.filter(function(n){return n.dispatchedAt===0||n.editedAt>n.dispatchedAt;}).length;}

  // ── hover highlight ────────────────────────────────────────
  document.addEventListener('mousemove',function(e){
    if(!hi||!enabled)return;var el=e.target;
    if(dialogOpen||isES(el)||anyWorking()){hi.style.display='none';return;}
    var r=el.getBoundingClientRect();
    Object.assign(hi.style,{display:'block',left:r.left+'px',top:r.top+'px',width:r.width+'px',height:r.height+'px'});
  },true);

  document.addEventListener('click',function(e){
    if(!enabled||anyWorking())return;var el=e.target;
    if(isES(el))return;
    e.preventDefault();e.stopPropagation();
    openInline(el);
  },true);

  // ── init ───────────────────────────────────────────────────
  function init(){load();ensure();renderBadges();listen();}
  window.__es={ensure:ensure,notes:function(){return notes;},enabled:function(){return enabled;}};
  init();
  setInterval(ensure,1000);
  // Fallback while the SSE stream is down (e.g. proxy briefly gone): keep
  // polling so a frozen overlay can still thaw if reconnects keep failing.
  setInterval(function(){if(proxyMode&&!sseLive)reconcile();},5000);
})();
true
`
