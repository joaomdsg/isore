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

  function isES(el){return !!(el&&el.closest&&el.closest('[data-es]'));}

  var proxyMode=!window.esDispatch;
  function ship(json){
    if(window.esDispatch){window.esDispatch(json);return;}
    fetch('/__eyesore/dispatch',{method:'POST',headers:{'Content-Type':'application/json'},body:json});
  }
  function listen(){
    if(!proxyMode||window.__esSSE)return;
    try{
      var es=new EventSource('/__eyesore/events');window.__esSSE=es;
      es.addEventListener('reload',function(){location.reload();});
      es.addEventListener('notes',function(ev){
        try{
          var changed=JSON.parse(ev.data)||[];
          var touched=false;
          changed.forEach(function(c){
            for(var i=0;i<notes.length;i++){
              if(notes[i].id===c.id){
                notes[i].agentStatus=c.agentStatus;
                notes[i].agentSummary=c.agentSummary;
                notes[i].fixedAt=c.fixedAt;
                touched=true;
              }
            }
          });
          if(touched){save();renderBadges();}
        }catch(_){}
      });
    }catch(_){}
  }

  function id(){return 'es_'+Date.now()+'_'+Math.random().toString(36).slice(2,6);}

  function esc(s){return(s||'').replace(/[&<>"]/g,function(c){return{'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c];});}

  function selectorOf(el){
    if(!el||el.nodeType!==1)return'';
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
    return parts.join(' > ');
  }

  // ── stylesheet (injected once) ─────────────────────────────
  function injectStyles(){
    if(document.getElementById('es-css'))return;
    var s=document.createElement('style');s.id='es-css';
    s.textContent=[
      '@keyframes es-fadeIn{from{opacity:0;transform:translateY(4px)}to{opacity:1;transform:translateY(0)}}',
      '@keyframes es-scaleIn{from{opacity:0;transform:scale(0.5)}to{opacity:1;transform:scale(1)}}',
      '@keyframes es-pulse{0%,100%{box-shadow:0 0 0 0 rgba(255,59,48,0)}50%{box-shadow:0 0 0 6px rgba(255,59,48,0.15)}}',
      '@keyframes es-glow{0%,100%{box-shadow:0 0 4px rgba(255,59,48,0.3)}50%{box-shadow:0 0 12px rgba(255,59,48,0.5)}}',
      '',
      '[data-es=hi]{position:fixed;pointer-events:none;z-index:2147483646;',
        'border:2px solid rgba(255,59,48,0.6);background:rgba(255,59,48,0.06);',
        'display:none;border-radius:4px;',
        'box-shadow:0 0 8px rgba(255,59,48,0.15);',
        'transition:all .12s ease-out}',
      '',
      '[data-es=toolbar]{position:fixed;right:16px;bottom:16px;z-index:2147483647;',
        'background:#1e1e1e;border-radius:12px;',
        'box-shadow:0 4px 20px rgba(0,0,0,.45),0 0 0 1px rgba(255,255,255,0.06);',
        'backdrop-filter:blur(8px);padding:12px 14px;',
        'width:fit-content;max-width:250px;',
        'animation:es-fadeIn .15s ease-out}',
      '[data-es=toolbar] .es-toolbar-row{display:flex;align-items:center;gap:8px}',
      '[data-es=toolbar] .es-toolbar-title{font:600 12px/1 system-ui,-apple-system,sans-serif;color:#e8e8e8;white-space:nowrap}',
      '[data-es=toolbar] .es-toolbar-desc{font:11px/1.35 system-ui,-apple-system,sans-serif;',
        'color:#777;margin:6px 0 10px}',
      '',
      '[data-es=toolbar] .es-switch{position:relative;width:36px;height:20px;cursor:pointer;flex-shrink:0}',
      '[data-es=toolbar] .es-switch input{opacity:0;width:0;height:0}',
      '[data-es=toolbar] .es-switch .es-slider{position:absolute;inset:0;',
        'background:#333;border-radius:20px;transition:all .2s ease}',
      '[data-es=toolbar] .es-switch .es-slider:before{content:"";position:absolute;',
        'width:16px;height:16px;border-radius:50%;left:2px;top:2px;',
        'background:#888;transition:all .2s ease}',
      '[data-es=toolbar] .es-switch input:checked+.es-slider{background:#1a7f4b}',
      '[data-es=toolbar] .es-switch input:checked+.es-slider:before{transform:translateX(16px);background:#fff}',
      '',
      '[data-es=toolbar] .es-toolbar-foot{margin-top:2px}',
      '[data-es=dispatch]{border:none;border-radius:7px;cursor:pointer;width:100%;',
        'font:600 12px/1 system-ui,-apple-system,sans-serif;',
        'color:#fff;background:#ff3b30;',
        'padding:8px 0;white-space:nowrap;',
        'transition:all .2s ease}',
      '[data-es=dispatch]:not(:disabled):hover{filter:brightness(1.15)}',
      '[data-es=dispatch]:not(:disabled):active{transform:scale(0.97)}',
      '[data-es=dispatch]:disabled{background:#2a2a2a;color:#555;cursor:default}',
      '[data-es=dispatch].es-green{background:#1a7f4b}',
      '',
      '[data-es=badge]{position:absolute;top:-8px;right:-8px;',
        'width:22px;height:22px;border-radius:50%;',
        'background:#ff3b30;color:#fff;',
        'display:flex;alignItems:center;justifyContent:center;',
        'font:700 11px/1 system-ui,-apple-system,sans-serif;',
        'cursor:pointer;z-index:2147483640;',
        'box-shadow:0 2px 8px rgba(0,0,0,.35);',
        'border:2px solid rgba(0,0,0,0.2);',
        'animation:es-scaleIn .2s ease-out;',
        'transition:all .15s ease;userSelect:none}',
      '[data-es=badge]:hover{transform:scale(1.2);animation:es-glow 1.5s ease-in-out infinite}',
      '',
      '[data-es=popover]{position:fixed;z-index:2147483645;',
        'background:#1e1e1e;color:#e0e0e0;',
        'padding:12px 16px;border-radius:10px;maxWidth:340px;minWidth:220px;',
        'font:13px/1.5 system-ui,-apple-system,sans-serif;',
        'box-shadow:0 12px 40px rgba(0,0,0,.5),0 0 0 1px rgba(255,255,255,0.06);',
        'backdrop-filter:blur(8px);',
        'animation:es-fadeIn .15s ease-out}',
      '[data-es=popover] .es-pop-note{margin-bottom:8px;white-space:pre-wrap;word-break:break-word;',
        'color:#e8e8e8}',
      '[data-es=popover] .es-pop-meta{font-size:11px;color:#666;margin-bottom:8px}',
      '[data-es=popover] .es-pop-agent{font-size:12px;color:#7fd6a4;margin-bottom:8px}',
      '[data-es=popover] .es-pop-actions{display:flex;gap:6px;justifyContent:flex-end}',
      '',
      '[data-es=popover] .es-btn{padding:5px 12px;font-size:12px;border-radius:5px;',
        'cursor:pointer;border:none;font:600 12px/1 system-ui,-apple-system,sans-serif;',
        'transition:all .15s ease}',
      '[data-es=popover] .es-btn:hover{filter:brightness(1.15)}',
      '[data-es=popover] .es-btn-edit{background:#2a2a2a;color:#bbb;border:1px solid #444}',
      '[data-es=popover] .es-btn-del{background:#3a1a1a;color:#e57373;border:1px solid #5a2a2a}',
      '[data-es=popover] .es-btn-cancel{background:#2a2a2a;color:#bbb;border:1px solid #444}',
      '[data-es=popover] .es-btn-confirm{background:#c62828;color:#fff}',
      '[data-es=popover] .es-btn-save{background:#1a7f4b;color:#fff}',
      '',
      '[data-es=inline]{position:fixed;z-index:2147483647;',
        'background:#1e1e1e;color:#e0e0e0;',
        'padding:14px 16px;border-radius:10px;width:290px;',
        'font:13px/1.4 system-ui,-apple-system,sans-serif;',
        'box-shadow:0 12px 40px rgba(0,0,0,.5),0 0 0 1px rgba(255,255,255,0.06);',
        'backdrop-filter:blur(8px);',
        'animation:es-fadeIn .15s ease-out}',
      '[data-es=inline] .es-inline-hint{font-size:11px;color:#666;margin-bottom:8px;',
        'white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
      '[data-es=inline] textarea{width:100%;box-sizing:border-box;',
        'font:13px/1.5 system-ui,-apple-system,sans-serif;',
        'padding:8px 10px;background:#141414;color:#e8e8e8;',
        'border:1px solid #333;border-radius:6px;resize:vertical;',
        'outline:none;transition:border-color .15s ease}',
      '[data-es=inline] textarea:focus{border-color:#ff3b30}',
      '[data-es=inline] textarea::placeholder{color:#555}',
      '[data-es=inline] .es-inline-actions{display:flex;gap:6px;justify-content:flex-end;margin-top:10px}',
      '[data-es=inline] .es-inline-shortcut{font-size:10px;color:#555;margin-top:6px;text-align:right}'
    ].join('');
    document.head.appendChild(s);
  }

  // ── persistence ────────────────────────────────────────────
  function storageKey(){return 'es_notes_'+location.href;}
  function save(){
    try{localStorage.setItem(storageKey(),JSON.stringify(notes));
        localStorage.setItem('es_enabled',enabled?'1':'0');}catch(_){}
  }
  function load(){
    try{var s=localStorage.getItem(storageKey());if(s)notes=JSON.parse(s)||[];
        var e=localStorage.getItem('es_enabled');enabled=e==='1';}catch(_){notes=[];}
  }

  // ── DOM scaffold ───────────────────────────────────────────
  var hi=null,toolbar=null,toggleInput=null,dispatchBtn=null;

  function ensure(){
    if(!document.body)return;
    injectStyles();
    if(!hi||!document.body.contains(hi)){
      hi=document.createElement('div');hi.setAttribute('data-es','hi');
      document.body.appendChild(hi);
    }
    if(!toolbar||!document.body.contains(toolbar)){
      toolbar=document.createElement('div');toolbar.setAttribute('data-es','toolbar');

      var row=document.createElement('div');row.className='es-toolbar-row';
      var sw=document.createElement('label');sw.className='es-switch';
      toggleInput=document.createElement('input');toggleInput.type='checkbox';
      toggleInput.addEventListener('change',function(ev){ev.stopPropagation();toggle();},true);
      var slider=document.createElement('span');slider.className='es-slider';
      sw.appendChild(toggleInput);sw.appendChild(slider);
      var title=document.createElement('span');title.className='es-toolbar-title';
      title.textContent='UI Annotator';
      row.appendChild(sw);row.appendChild(title);

      var desc=document.createElement('div');desc.className='es-toolbar-desc';
      desc.innerHTML='Click elements to annotate.';

      var foot=document.createElement('div');foot.className='es-toolbar-foot';
      dispatchBtn=document.createElement('button');dispatchBtn.setAttribute('data-es','dispatch');
      dispatchBtn.disabled=true;
      dispatchBtn.addEventListener('click',function(ev){ev.preventDefault();ev.stopPropagation();dispatch();},true);
      foot.appendChild(dispatchBtn);

      toolbar.appendChild(row);toolbar.appendChild(desc);toolbar.appendChild(foot);
      document.body.appendChild(toolbar);
    }
    applyEnabled();
  }

  // ── toggle ─────────────────────────────────────────────────
  function toggle(){
    enabled=!enabled;save();applyEnabled();
    if(window.esToggle)window.esToggle(enabled?'on':'off');
  }
  function applyEnabled(){
    if(toggleInput)toggleInput.checked=enabled;
    if(dispatchBtn){
      var c=countEdited();
      var hasNotes=notes.length>0;
      dispatchBtn.textContent=enabled?('Dispatch ('+c+')'):'Dispatch';
      dispatchBtn.disabled=!enabled||!hasNotes;
      dispatchBtn.className=c>0&&enabled?'es-green':'';
    }
    if(hi)hi.style.display='none';
    renderBadges();
  }

  // ── badges ─────────────────────────────────────────────────
  function renderBadges(){
    document.querySelectorAll('[data-es=badge]').forEach(function(b){b.remove();});
    if(!enabled)return;
    notes.forEach(function(n,i){
      try{
        var el=document.querySelector(n.selector);if(!el)return;
        if(!el.style.position||el.style.position==='static')el.style.position='relative';
        var badge=document.createElement('div');badge.setAttribute('data-es','badge');
        badge.textContent=String(i+1);
        if(n.agentStatus==='working')badge.style.background='#f5a623';
        if(n.agentStatus==='fixed')badge.style.background='#1a7f4b';
        badge.addEventListener('mouseenter',function(ev){ev.stopPropagation();showPopover(el,n);},true);
        badge.addEventListener('mouseleave',function(){scheduleHidePopover();},true);
        el.appendChild(badge);
      }catch(_){}
    });
  }

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
      '<button data-es="pop-del" class="es-btn es-btn-del">Delete</button></div>';
    pop.addEventListener('mouseenter',function(){clearTimeout(popTimer);},true);
    pop.addEventListener('mouseleave',function(){scheduleHidePopover();},true);
    pop.querySelector('[data-es=pop-edit]').addEventListener('click',function(ev){ev.stopPropagation();hidePopover();openInlineEdit(el,note);},true);
    pop.querySelector('[data-es=pop-del]').addEventListener('click',function(ev){ev.stopPropagation();confirmDelete(el,note,pop);},true);
    document.body.appendChild(pop);popEl=pop;
  }
  function hidePopover(){if(popEl){popEl.remove();popEl=null;}}
  function scheduleHidePopover(){popTimer=setTimeout(hidePopover,250);}

  // ── delete confirm ─────────────────────────────────────────
  function confirmDelete(el,note,pop){
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
    if(dialogOpen)return;dialogOpen=true;
    if(hi)hi.style.display='none';
    var card=buildInlineCard(el,selectorOf(el));
    card.innerHTML='<div class="es-inline-hint">'+esc(selectorOf(el))+'</div>'+
      '<textarea data-es="in" rows="3" placeholder="Add a note\u2026"></textarea>'+
      '<div class="es-inline-actions">'+
      '<button data-es="cancel" class="es-btn es-btn-cancel">Cancel</button>'+
      '<button data-es="add" class="es-btn es-btn-save">Add</button></div>'+
      '<div class="es-inline-shortcut">\u2318\u21E7\u23CE to save \u00b7 Esc to cancel</div>';
    document.body.appendChild(card);
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
    if(dialogOpen)return;dialogOpen=true;
    var card=buildInlineCard(el);
    card.innerHTML='<div class="es-inline-hint">Edit note</div>'+
      '<textarea data-es="in" rows="3" placeholder="Note text\u2026">'+esc(note.note)+'</textarea>'+
      '<div class="es-inline-actions">'+
      '<button data-es="cancel" class="es-btn es-btn-cancel">Cancel</button>'+
      '<button data-es="save" class="es-btn es-btn-save">Save</button></div>'+
      '<div class="es-inline-shortcut">\u2318\u21E7\u23CE to save \u00b7 Esc to cancel</div>';
    document.body.appendChild(card);
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
    if(!enabled||notes.length===0)return;
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
    if(dialogOpen||isES(el)){hi.style.display='none';return;}
    var r=el.getBoundingClientRect();
    Object.assign(hi.style,{display:'block',left:r.left+'px',top:r.top+'px',width:r.width+'px',height:r.height+'px'});
  },true);

  document.addEventListener('click',function(e){
    if(!enabled)return;var el=e.target;
    if(isES(el))return;
    e.preventDefault();e.stopPropagation();
    openInline(el);
  },true);

  // ── init ───────────────────────────────────────────────────
  function init(){load();ensure();renderBadges();listen();}
  window.__es={ensure:ensure,notes:function(){return notes;},enabled:function(){return enabled;}};
  init();
  setInterval(ensure,1000);
})();
true
`
