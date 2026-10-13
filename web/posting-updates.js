/* Stored-state observation only. This file never opens an operation endpoint.
   Keyed reconciliation retains cards, controls and disclosures; server order is
   authoritative, including selected archive day groups and capped backfill. */
(function () {
  'use strict';
  function revision(value) {
    if(typeof value!=='string'||! /^(0|[1-9][0-9]{0,19})$/.test(value))return null;
    var n=BigInt(value);return n<=18446744073709551615n?n:null;
  }
  function viewAdmission(view,request,last,current) {
    if(!view||request.owner!==current.owner||request.contextSequence!==current.contextSequence||
       view.schema!=='posting-view-v1'||view.generation!==request.generation||view.surface!==request.surface||view.sort!==request.sort||
       typeof view.epoch!=='string'||!view.epoch||typeof view.context!=='string'||!view.context||!Array.isArray(view.rows)||!Array.isArray(view.sections))return false;
    var g=revision(view.global_revision),u=revision(view.user_revision);
    if(g===null||u===null||(current.retired||[]).indexOf(view.epoch)!==-1)return false;
    if(current.epoch&&view.epoch!==current.epoch&&!request.force)return false;
    if(last&&view.epoch!==last.epoch&&!request.force)return false;
    // A context-triggered read remains locked to the notice. A later deliberate,
    // periodic or reconnect baseline may discover a superseding external write,
    // but only if started after that notice and at/after its committed revision.
    if(current.context&&view.context!==current.context&&
       (!request.force||(current.contextLocked&&(!request.baseline||u<revision(current.contextRevision||'0')))))return false;
    if(last&&view.epoch===last.epoch&&(g<revision(last.global_revision)||u<revision(last.user_revision)))return false;
    return ['total','main','excluded'].every(function(key){return Number.isInteger(view[key])&&view[key]>=0;}) && view.rows.length===view.main+view.excluded;
  }
  function createRunSummary(doc,node) {
    var epoch='',latest=new Map(),dismissed=new Map(),entries=[],signature='';
    function render() {
      var shown=entries.filter(function(entry){return dismissed.get(entry.surface)!==entry.key;});
      var next=JSON.stringify(shown);
      if(next===signature)return;
      signature=next;node.textContent='';
      shown.forEach(function(entry,index){
        if(index) {var separator=doc.createElement('span');separator.textContent=' · ';node.appendChild(separator);}
        var item=doc.createElement('span'),text=doc.createElement('span');text.textContent=entry.message;item.appendChild(text);
        if(entry.warning) {
          var button=doc.createElement('button');button.type='button';button.textContent='알림 닫기';
          button.addEventListener('click',function(){dismissed.set(entry.surface,entry.key);render();});item.appendChild(button);
        }
        node.appendChild(item);
      });
    }
    return function(view) {
      if(!node)return;
      if(epoch!==view.epoch){epoch=view.epoch;latest.clear();dismissed.clear();}
      entries=[];
      ['today','bookmarks','archive'].forEach(function(surface){
        var run=(view.runs||{})[surface];
        if(!run||!Number.isSafeInteger(run.run_id)||run.run_id<1||!run.run_token)return;
        if(run.run_id<(latest.get(surface)||0))return;
        latest.set(surface,run.run_id);
        var warning=run.state==='failed'||(run.state==='done'&&(run.outcome==='partial'||run.outcome==='no_progress'));
        var message=run.state==='running'?(run.progress||run.status||'공고를 분석하는 중...'):warning?run.message:'';
        // Three scoped transient summaries maximum; no owner completion replay
        // or saved outcome inferred from an uncommitted provider response.
        if(message)entries.push({surface:surface,key:epoch+':'+run.run_token,warning:warning,message:message});
      });
      render();
    };
  }
  function init() {
    var root = document.getElementById('posting-lists');
    if (!root || document.body.dataset.demo === 'true') return;
    var surface = document.body.dataset.liveSurface;
    var sort = document.body.dataset.liveSort || 'date';
    var notice = document.getElementById('posting-live-notice');
    var lastHTML = new Map();
    var animations = new Map();
    var updateRunSummary=createRunSummary(document,document.getElementById('posting-run-summary'));


    function visible(el) {
      if (!el || !el.isConnected || !el.getClientRects().length) return false;
      var node = el;
      while (node && node !== root.parentNode) {
        if (node.hidden || node.classList.contains('filter-hidden')) return false;
        if (node.tagName === 'DETAILS' && !node.open && !node.querySelector('summary').contains(el)) return false;
        node = node.parentElement;
      }
      return true;
    }
    function controlKind(el) {
      return ['posting-title','bookmark','not-interested','chip-ai'].find(function (name) {return el && el.classList.contains(name);});
    }
    function morph(old, next) {
      if (old.nodeType !== next.nodeType || old.nodeName !== next.nodeName) {old.replaceWith(next.cloneNode(true)); return;}
      if (old.nodeType === 3) {if (old.nodeValue !== next.nodeValue) old.nodeValue = next.nodeValue; return;}
      if (old.nodeType !== 1) return;
      var pendingControl = old.tagName === 'BUTTON' && old.disabled;
      Array.from(old.attributes).forEach(function (attr) {
        if (attr.name === 'open' || attr.name === 'disabled' || attr.name === 'aria-expanded' || (pendingControl && (attr.name === 'class' || attr.name === 'aria-pressed'))) return;
        if (!next.hasAttribute(attr.name)) old.removeAttribute(attr.name);
      });
      Array.from(next.attributes).forEach(function (attr) {
        if (attr.name === 'open' || attr.name === 'disabled' || attr.name === 'aria-expanded' || (pendingControl && (attr.name === 'class' || attr.name === 'aria-pressed'))) return;
        if (old.getAttribute(attr.name) !== attr.value) old.setAttribute(attr.name,attr.value);
      });
      var a = Array.from(old.childNodes), b = Array.from(next.childNodes);
      for (var i=0;i<Math.max(a.length,b.length);i++) {
        if (!a[i]) old.appendChild(b[i].cloneNode(true));
        else if (!b[i]) a[i].remove();
        else morph(a[i],b[i]);
      }
    }
    function ensureSection(section, existing) {
      var node = existing.get(section.key);
      if (!node) {
        if (section.excluded) {
          node = document.createElement('details'); node.className = 'excluded-box';
          var summary = document.createElement('summary'); summary.appendChild(document.createTextNode('관심 밖으로 분류된 공고 '));
          var count = document.createElement('span'); count.className = 'excluded-count'; summary.appendChild(count);summary.appendChild(document.createTextNode('개'));node.appendChild(summary);
        } else if (section.title) {
          node = document.createElement('section'); node.className = 'archive-day';
          var heading = document.createElement('h2'); heading.className = 'day-header';heading.textContent=section.title;
          if (section.is_today) {var today=document.createElement('span');today.className='day-today';today.textContent='오늘';heading.appendChild(document.createTextNode(' '));heading.appendChild(today);}
          node.appendChild(heading);
        } else {node = document.createElement('ol');node.className='postings';}
        node.dataset.liveSection=section.key;
        if (node.tagName !== 'OL') {var list=document.createElement('ol');list.className='postings';node.appendChild(list);}
      }
      return node;
    }
    function reconcile(view) {
      // Parse/validate ALL fragments before changing the last good DOM.
      var parsed = new Map(), ids = new Set(), sections = new Map();
      view.sections.forEach(function (section) {if (!section.key || sections.has(section.key)) throw Error('section');sections.set(section.key,section);});
      view.rows.forEach(function (row) {
        if (!/^\d+$/.test(row.id) || ids.has(row.id) || !sections.has(row.section) || typeof row.html !== 'string') throw Error('row');
        var template = document.createElement('template');template.innerHTML=row.html;
        var node=template.content.firstElementChild;
        if (!node || node.tagName!=='LI' || node.dataset.postingKey!==row.id || template.content.children.length!==1) throw Error('fragment');
        parsed.set(row.id,node);ids.add(row.id);
      });
      var cards = Array.from(root.querySelectorAll('[data-posting-key]'));
      var keyed = new Map(cards.map(function (card) {return [card.dataset.postingKey,card];}));
      var focus=document.activeElement, focusedCard=focus && focus.closest('[data-posting-key]'), kind=controlKind(focus);
      var anchor=cards.find(function(card){return ids.has(card.dataset.postingKey)&&visible(card)&&card.getBoundingClientRect().bottom>0;});
      var offset=anchor?anchor.getBoundingClientRect().top:0;
      var positions=new Map(cards.filter(visible).map(function(card){return [card.dataset.postingKey,card.getBoundingClientRect().top];}));
      var readCard=focusedCard || anchor;
      var oldSections=new Map(cards.map(function(card){var group=card.closest('[data-live-section]');return [card.dataset.postingKey,group?group.dataset.liveSection:''];}));
      var existing=new Map(Array.from(root.querySelectorAll('[data-live-section]')).map(function(node){return [node.dataset.liveSection,node];}));
      var destinations=new Map();
      view.sections.forEach(function(section,index){var node=ensureSection(section,existing);var current=Array.from(root.children).filter(function(child){return child.dataset.liveSection;})[index];if(current!==node)root.insertBefore(node,current||null);node.hidden=false;destinations.set(section.key,node.tagName==='OL'?node:node.querySelector('ol'));existing.delete(section.key);});
      var sectionOffsets=new Map();
      view.rows.forEach(function(row){
        var card=keyed.get(row.id),next=parsed.get(row.id);
        if (!card) {card=next;keyed.set(row.id,card);}
        else if (lastHTML.get(row.id)!==row.html) morph(card,next);
        if (card.querySelector('button:disabled')) lastHTML.delete(row.id);else lastHTML.set(row.id,row.html);
        var list=destinations.get(row.section),index=sectionOffsets.get(row.section)||0;
        if(list.children[index]!==card)list.insertBefore(card,list.children[index]||null);
        sectionOffsets.set(row.section,index+1);
        if (readCard===card && row.section==='excluded' && oldSections.get(row.id)!=='excluded' && notice) notice.textContent='점수가 바뀌어 관심 밖으로 옮겼어요.';
      });
      cards.forEach(function(card){if(!ids.has(card.dataset.postingKey)){lastHTML.delete(card.dataset.postingKey);card.remove();}});
      existing.forEach(function(node){node.remove();});
      var excluded=root.querySelector('.excluded-box');
      if (excluded) {excluded.hidden=view.excluded===0;var count=excluded.querySelector('.excluded-count');if(count) count.textContent=String(view.excluded);}
      root.querySelectorAll('.empty').forEach(function(node){node.hidden=true;});
      var empty=root.querySelector('[data-live-empty]');
      if (!empty) {empty=document.createElement('div');empty.className='empty';empty.dataset.liveEmpty='true';var p=document.createElement('p');p.className='empty-title';p.tabIndex=-1;empty.appendChild(p);root.appendChild(empty);}
      empty.hidden=view.main!==0;
      empty.firstElementChild.textContent=view.profile_required?'프로필을 설정하면 공고를 보여드릴게요.':surface==='bookmarks'?'아직 저장한 공고가 없어요.':surface==='today'?'오늘은 아직 브리핑할 공고가 없어요.':'아직 스크랩한 공고가 없어요.';
      var total=document.querySelector('.count strong');if(total) total.textContent=String(surface==='today'?view.main:view.total);
      // Filters must refresh their registry, but live movement MUST NOT invoke
      // the explicit search-input autoopen policy.
      document.dispatchEvent(new CustomEvent('posting-live-change',{detail:{excluded:view.excluded}}));
      root.querySelectorAll('.chip-ai[aria-expanded="true"]').forEach(function(btn){var ev=btn.parentElement.querySelector('.ai-evidence');if(!visible(btn)){btn.setAttribute('aria-expanded','false');if(ev)ev.hidden=true;}else if(ev)ev.hidden=false;});
      if (anchor && visible(anchor)) window.scrollBy(0,anchor.getBoundingClientRect().top-offset);
      if (focusedCard && (!visible(focus) || !focus.isConnected || document.activeElement!==focus)) {
        var same=keyed.get(focusedCard.dataset.postingKey), target=kind&&same?same.querySelector('.'+kind):null;
        var nowCards=Array.from(root.querySelectorAll('[data-posting-key]')).filter(visible);
        if (!visible(target)) {var index=cards.indexOf(focusedCard);target=cards.slice(index+1).concat(cards.slice(0,index).reverse()).map(function(card){return keyed.get(card.dataset.postingKey);}).find(visible);target=target?target.querySelector('.posting-title'):null;}
        if (!visible(target) && nowCards.length) target=nowCards[0].querySelector('.posting-title');
        if (!visible(target)) target=excluded&&!excluded.hidden?excluded.querySelector('summary'):empty.firstElementChild;
        if (target) target.focus({preventScroll:true});
      }
      animations.forEach(function(animation){animation.cancel();});animations.clear();
      if(!window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
        view.rows.forEach(function(row){var card=keyed.get(row.id),previous=positions.get(row.id);if(previous===undefined||!visible(card)||!card.animate)return;
          var dy=previous-card.getBoundingClientRect().top;if(Math.abs(dy)<1)return;
          var animation=card.animate([{transform:'translateY('+dy+'px)'},{transform:'none'}],{duration:140,easing:'ease-out'});animations.set(row.id,animation);
          animation.onfinish=function(){if(animations.get(row.id)===animation)animations.delete(row.id);};
        });
      }
      updateCounts(view);
    }
    function updateCounts(view) {
      var info=view.rerate, button=document.getElementById('rerate'), row=document.querySelector('.rerate-row');
      if (row) row.hidden=!info;
      var feedback=document.querySelector('.rerate-feedback');if(feedback)feedback.hidden=!info;
      if (info && button) {button.classList.toggle('has-stale',info.PendingCount>0);button.textContent='AI 평가'+(info.PendingCount?' ·'+info.PendingCount:'');}
      var hint=document.querySelector('.rerate-hint');
      if(hint)hint.textContent=info?'AI로 화면에 보이는 공고를 분석해요'+(info.PendingContextCount?' · AI 문맥 확인이 필요한 공고 '+info.PendingContextCount+'개':'')+(info.PendingScoreCount?' · 다시 평가할 공고 '+info.PendingScoreCount+'개':''):'';
      var count=document.querySelector('.rerate-count');
      if (!count && row && info) {count=document.createElement('span');count.className='rerate-count';row.appendChild(count);}
      if(count) {count.hidden=!info||!info.Visible;count.textContent=info?'AI 분석 '+info.Analyzed+'/'+info.Visible:'';count.title=info?'화면에 보이는 공고 '+info.Visible+'개 중 '+info.Analyzed+'개를 AI로 분석했어요. 토큰을 아끼려고 한 번에 일정 개수만 분석하고, 더 보려면 AI 평가를 다시 누르면 돼요.':'';count.classList.toggle('complete',!!info&&info.Visible>0&&info.Analyzed>=info.Visible);}
      updateRunSummary(view);
    }
    var engine=createObserver({now:Date.now,setTimeout:window.setTimeout.bind(window),clearTimeout:window.clearTimeout.bind(window),fetch:window.fetch.bind(window),EventSource:window.EventSource,AbortController:window.AbortController,
      visible:function(){return document.visibilityState==='visible'&&navigator.onLine!==false;}},surface,sort,function(view){reconcile(view);document.dispatchEvent(new CustomEvent('posting-view-applied',{detail:view}));},function(){
        root.textContent='';var text=document.createElement('p');text.textContent='다시 로그인하면 저장된 결과를 확인할 수 있어요.';var link=document.createElement('a');link.href='/login';link.textContent='로그인';root.appendChild(text);root.appendChild(link);
        document.querySelectorAll('.rerate-row,.rerate-feedback').forEach(function(node){node.hidden=true;node.textContent='';});var summary=document.getElementById('posting-run-summary');if(summary)summary.textContent='';
      },function(stale){var node=document.getElementById('posting-live-stale');if(node)node.hidden=!stale;});
    window.JobcronPostingUpdates={resync:engine.force};
    var retry=document.getElementById('posting-live-retry');if(retry)retry.addEventListener('click',engine.force);
    document.addEventListener('visibilitychange',function(){if(document.visibilityState==='visible')engine.resume();else engine.pause();});
    window.addEventListener('pagehide',engine.pause);window.addEventListener('pageshow',engine.resume);
    window.addEventListener('offline',engine.pause);window.addEventListener('online',engine.resume);
    engine.resume();
  }

  // The clock/transport seam is only for deterministic zero-package tests. The
  // browser supplies its native APIs; no provider/runtime or operation URL exists.
  function createObserver(env,surface,sort,onView,onAuth,onStale) {
    var applied=null,generation=0,pending=null,dirty=false,forced=false,baseline=false,active=false,authLost=false;
    var timer=null,periodic=null,poll=null,reconnect=null,source=null,lastStart=-Infinity,failures=0,observeFailures=0;
    var admission={owner:0,contextSequence:0,epoch:'',context:'',contextLocked:false,contextRevision:'0',user_revision:'0',retired:[]};
    function allowed(){return active&&!authLost&&env.visible();}
    function clear(name){if(name!==null)env.clearTimeout(name);}
    function cancelRead(){var old=pending;pending=null;if(old){clear(old.deadline);old.controller.abort();}}
    function schedule(){
      if(!allowed()||pending||timer!==null)return;
      timer=env.setTimeout(function(){timer=null;read();},Math.max(0,1000-(env.now()-lastStart)));
    }
    function resync(force){dirty=true;forced=forced||!!force;schedule();}
    function force(){baseline=true;resync(true);}
    function release(record){clear(record.deadline);if(pending===record)pending=null;}
    function fail(record){
      if(pending!==record||record.owner!==admission.owner)return;
      release(record);record.controller.abort();failures++;onStale(failures>=5);
      clear(poll);poll=env.setTimeout(function(){poll=null;force();},Math.min(30000,5000*Math.pow(2,Math.min(failures-1,3))));
    }
    function read(){
      if(!allowed()||pending)return;
      dirty=false;lastStart=env.now();generation++;
      var record={generation:String(generation),surface:surface,sort:sort,owner:admission.owner,contextSequence:admission.contextSequence,force:forced,baseline:baseline,controller:new env.AbortController()};forced=false;baseline=false;pending=record;
      record.deadline=env.setTimeout(function(){fail(record);},10000);
      env.fetch('/api/posting-view?surface='+encodeURIComponent(surface)+'&sort='+encodeURIComponent(sort)+'&generation='+record.generation,{cache:'no-store',signal:record.controller.signal,headers:{Accept:'application/json'}})
        .then(function(response){
          if(pending!==record||record.owner!==admission.owner)return null;
          if(response.status===401){authLost=true;pause();onAuth();return null;}
          if(!response.ok)throw Error('read');return response.json();
        }).then(function(view){
          if(pending!==record||record.owner!==admission.owner)return;
          if(!viewAdmission(view,record,applied,admission))throw Error('view');
          onView(view);
          if(applied&&applied.epoch!==view.epoch)admission.retired=[applied.epoch];
          applied=view;admission.epoch=view.epoch;admission.context=view.context;admission.contextLocked=false;
          failures=0;onStale(false);release(record);
          if(dirty)schedule();
          if(!source)armPoll(5000);
        }).catch(function(){fail(record);});
    }
    function hint(n){
      if(!allowed()||!n||typeof n.epoch!=='string'||!n.epoch)return;
      if(admission.retired.indexOf(n.epoch)!==-1)return;
      var u=revision(n.user_revision);
      if((n.context||n.context_changed)&&admission.epoch===n.epoch&&u!==null&&
         (u<revision(admission.user_revision)||u<=revision(admission.contextRevision)||(applied&&applied.epoch===n.epoch&&u<=revision(applied.user_revision)))) {
        // A coalesced notice may still carry newer public changes or status.
        // Ignore only its obsolete context, not that independent dirty signal.
        n=Object.assign({},n,{context:'',context_changed:false});
      }
      var changed=!!n.context_changed||(admission.epoch&&admission.epoch!==n.epoch)||(n.context&&admission.context&&admission.context!==n.context);
      if(changed){admission.contextSequence++;cancelRead();baseline=false;}
      if(admission.epoch!==n.epoch){if(admission.epoch)admission.retired=[admission.epoch];admission.context='';admission.user_revision='0';admission.contextRevision='0';}
      admission.epoch=n.epoch;
      if(changed&&u!==null)admission.contextRevision=n.user_revision;
      if(n.context_changed&&!n.context){admission.context='';admission.contextLocked=false;}
      if(n.context){admission.context=n.context;admission.contextLocked=true;admission.contextRevision=u===null?admission.user_revision:n.user_revision;}
      if(revision(n.user_revision)!==null&&revision(n.user_revision)>revision(admission.user_revision))admission.user_revision=n.user_revision;
      resync(!!n.resync||changed);
    }
    function armPoll(delay){clear(poll);if(!allowed())return;poll=env.setTimeout(function(){poll=null;force();},delay);}
    function connect(){
      if(!allowed()||source)return;
      var next=new env.EventSource('/api/posting-updates');source=next;
      next.addEventListener('open',function(){if(source===next){observeFailures=0;force();}});
      next.addEventListener('dirty',function(event){if(source!==next)return;try{hint(JSON.parse(event.data));}catch(error){force();}});
      next.addEventListener('error',function(){
        if(source!==next)return;next.close();source=null;observeFailures++;force();armPoll(5000);
        clear(reconnect);reconnect=env.setTimeout(function(){reconnect=null;connect();},Math.min(30000,5000*Math.pow(2,Math.min(observeFailures-1,3))));
      });
    }
    function periodicRead(){clear(periodic);if(!allowed())return;periodic=env.setTimeout(function(){periodic=null;force();periodicRead();},30000);}
    function pause(){
      active=false;admission.owner++;cancelRead();clear(timer);clear(periodic);clear(poll);clear(reconnect);timer=periodic=poll=reconnect=null;
      if(source)source.close();source=null;
    }
    function resume(){
      if(authLost||!env.visible())return;
      if(active){force();return;}
      active=true;admission.owner++;admission.epoch='';admission.context='';admission.contextLocked=false;connect();periodicRead();force();
    }
    return {resume:resume,pause:pause,force:force,hint:hint};
  }
  if(typeof module==='object'&&module.exports)module.exports={viewAdmission:viewAdmission,createObserver:createObserver,createRunSummary:createRunSummary};
  if(typeof document!=='undefined'){if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',init);else init();}
})();
