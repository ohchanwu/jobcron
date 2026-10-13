'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');const path=require('node:path');const vm=require('node:vm');
const box={module:{exports:{}},document:{readyState:'loading',addEventListener(){}}};
vm.runInNewContext(fs.readFileSync(path.join(__dirname,'../posting-updates.js'),'utf8'),box);
const create=box.module.exports.createObserver;
const flush=async()=>{for(let i=0;i<12;i++)await Promise.resolve();};
function deferred(){let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b;});return {promise,resolve,reject};}
function harness(){
 let time=0,next=0,visible=true;const timers=new Map(),sources=[],reads=[],views=[],stale=[],auth=[];
 class Source{constructor(url){this.url=url;this.handlers={};this.closed=false;sources.push(this);}addEventListener(name,fn){this.handlers[name]=fn;}close(){this.closed=true;}emit(name,value){if(this.handlers[name])this.handlers[name]({data:JSON.stringify(value)});}}
 const env={now:()=>time,setTimeout:(fn,delay)=>{const id=++next;timers.set(id,{fn,time:time+delay});return id;},clearTimeout:id=>timers.delete(id),visible:()=>visible,EventSource:Source,AbortController,
  fetch:(url,options)=>{const result=deferred();reads.push({url,options,...result});return result.promise;}};
 const c=create(env,'archive','date',v=>views.push(v),()=>auth.push(true),v=>stale.push(v));
 async function advance(ms){const end=time+ms;for(;;){let pair=[...timers].sort((a,b)=>a[1].time-b[1].time)[0];if(!pair||pair[1].time>end)break;time=pair[1].time;timers.delete(pair[0]);pair[1].fn();await flush();}time=end;await flush();}
 function snapshot(read,extra={}){return {schema:'posting-view-v1',epoch:'e1',global_revision:'1',user_revision:'1',context:'c1',generation:new URL(read.url,'http://local.invalid').searchParams.get('generation'),surface:'archive',sort:'date',rows:[],sections:[],total:0,main:0,excluded:0,...extra};}
 return {c,env,sources,reads,views,stale,auth,advance,snapshot,hide(){visible=false;c.pause();},show(){visible=true;c.resume();}};
}
(async()=>{
 const h=harness();h.c.resume();await h.advance(0);assert.equal(h.sources.length,1);assert.equal(h.reads.length,1);
 h.sources[0].emit('dirty',{epoch:'e1',global_revision:'99',user_revision:'1'});
 h.reads[0].resolve({ok:true,status:200,json:async()=>h.snapshot(h.reads[0])});await flush();assert.equal(h.views.length,1,'ordinary dirty cannot starve a complete view');await h.advance(1000);assert.equal(h.reads.length,2);
 const body=deferred();h.reads[1].resolve({ok:true,status:200,json:()=>body.promise});await flush();h.hide();assert.equal(h.reads[1].options.signal.aborted,true);assert.equal(h.sources[0].closed,true);
 body.resolve(h.snapshot(h.reads[1],{global_revision:'2'}));await flush();assert.equal(h.views.length,1,'late hidden body');await h.advance(60000);assert.equal(h.reads.length,2,'no hidden polling');
 h.show();await h.advance(0);assert.equal(h.sources.length,2);assert.equal(h.reads.length,3,'visibility return reads stored state');
 h.reads[2].resolve({ok:true,status:200,json:async()=>h.snapshot(h.reads[2],{epoch:'e2',context:'c2'})});await flush();assert.equal(h.views.length,2,'restart is a forced baseline, not a completed run');
 h.sources[1].emit('error',{});assert.equal(h.sources[1].closed,true,'disable native observe reconnect');await h.advance(1000);
 for(let i=0;i<5;i++){const read=h.reads.at(-1);read.reject(Error('synthetic read fault'));await flush();await h.advance(30000);}
 assert.ok(h.stale.includes(true),'five failed reads show explicit stale state');
 const unauthorized=h.reads.at(-1);unauthorized.resolve({status:401,ok:false,json:async()=>({})});await flush();assert.equal(h.auth.length,1);
 const count=h.reads.length;await h.advance(60000);assert.equal(h.reads.length,count,'authentication loss closes read recovery');
 assert.ok(h.reads.every(r=>r.url.startsWith('/api/posting-view?')));assert.ok(h.sources.every(s=>s.url==='/api/posting-updates'));
 const epoch=harness();epoch.c.resume();await epoch.advance(0);
 epoch.reads[0].resolve({ok:true,status:200,json:async()=>epoch.snapshot(epoch.reads[0])});await flush();
 epoch.c.hint({epoch:'e2',context:'c2',resync:true});await epoch.advance(1000);
 epoch.reads[1].resolve({ok:true,status:200,json:async()=>epoch.snapshot(epoch.reads[1],{epoch:'e2',context:'c2'})});await flush();
 epoch.c.hint({epoch:'e1',context:'c1',resync:true});await epoch.advance(1000);
 assert.equal(epoch.reads.length,2,'a delayed retired-epoch notice cannot roll current authority backward');epoch.c.pause();
 const unknown=harness();unknown.c.resume();await unknown.advance(0);
 unknown.reads[0].resolve({ok:true,status:200,json:async()=>unknown.snapshot(unknown.reads[0])});await flush();
 unknown.c.force();await unknown.advance(1000);
 unknown.c.hint({epoch:'e1',context_changed:true,resync:true,user_revision:'2'});
 assert.equal(unknown.reads[1].options.signal.aborted,true,'a committed context with unavailable metadata still invalidates old reads');
 unknown.c.hint({epoch:'e1',global_revision:'3'});await unknown.advance(1000);
 unknown.reads[2].resolve({ok:true,status:200,json:async()=>unknown.snapshot(unknown.reads[2],{context:'c2',user_revision:'2'})});await flush();
 assert.equal(unknown.views.at(-1).context,'c2','unknown context must recover from real metadata, never lock to an invented token');unknown.c.pause();
 const superseded=harness();superseded.c.resume();await superseded.advance(0);
 superseded.reads[0].resolve({ok:true,status:200,json:async()=>superseded.snapshot(superseded.reads[0])});await flush();
 superseded.c.force();await superseded.advance(1000);const oldBody=deferred();
 superseded.reads[1].resolve({ok:true,status:200,json:()=>oldBody.promise});await flush();
 superseded.c.hint({epoch:'e1',context:'c2',context_changed:true,user_revision:'2'});
 assert.equal(superseded.reads[1].options.signal.aborted,true);
 oldBody.resolve(superseded.snapshot(superseded.reads[1],{context:'c1',user_revision:'2'}));await flush();
 await superseded.advance(1000);
 superseded.reads[2].resolve({ok:true,status:200,json:async()=>superseded.snapshot(superseded.reads[2],{context:'c3',user_revision:'2'})});await flush();
 assert.equal(superseded.views.length,1,'context-triggered read retains the known-context gate');
 superseded.c.force();await superseded.advance(1000);
 superseded.reads.at(-1).resolve({ok:true,status:200,json:async()=>superseded.snapshot(superseded.reads.at(-1),{context:'c3',user_revision:'2'})});await flush();
 assert.equal(superseded.views.at(-1).context,'c3','explicit baseline recovers a superseded announced context after external writes');
 superseded.c.hint({epoch:'e1',context:'c2',context_changed:true,user_revision:'2'});
 superseded.c.hint({epoch:'e1',context:'c1',context_changed:true,user_revision:'1'});
 superseded.c.force();await superseded.advance(1000);
 const duplicateRead=superseded.reads.at(-1);
 duplicateRead.resolve({ok:true,status:200,json:async()=>superseded.snapshot(duplicateRead,{context:'c3',user_revision:'2'})});await flush();
 assert.equal(superseded.views.at(-1).context,'c3','out-of-order/duplicate hints cannot roll context authority back');
 superseded.c.hint({epoch:'e1',context:'c4',context_changed:true,user_revision:'4'});await superseded.advance(1000);
 superseded.c.force(); // A force requested DURING an old read cannot grant it baseline authority.
 superseded.reads.at(-1).resolve({ok:true,status:200,json:async()=>superseded.snapshot(superseded.reads.at(-1),{context:'c3',user_revision:'2'})});await flush();
 assert.equal(superseded.views.at(-1).context,'c3');
 await superseded.advance(30000);
 superseded.reads.at(-1).resolve({ok:true,status:200,json:async()=>superseded.snapshot(superseded.reads.at(-1),{context:'c5',user_revision:'4'})});await flush();
 assert.equal(superseded.views.at(-1).context,'c5','periodic post-notice authoritative read recovers without revision rollback');superseded.c.pause();
 const ordered=harness();ordered.c.resume();await ordered.advance(0);
 ordered.reads[0].resolve({ok:true,status:200,json:async()=>ordered.snapshot(ordered.reads[0])});await flush();
 ordered.c.hint({epoch:'e1',context:'c3',context_changed:true,user_revision:'3'});await ordered.advance(1000);
 const currentRead=ordered.reads.at(-1);
 ordered.c.hint({epoch:'e1',context:'c3',context_changed:true,user_revision:'3'});
 ordered.c.hint({epoch:'e1',context:'c2',context_changed:true,user_revision:'2',global_revision:'100'});
 assert.equal(currentRead.options.signal.aborted,false,'duplicate/older context hints cannot invalidate the current post-notice read');
 currentRead.resolve({ok:true,status:200,json:async()=>ordered.snapshot(currentRead,{context:'c3',user_revision:'3'})});await flush();
 assert.equal(ordered.views.at(-1).context,'c3');ordered.c.pause();
 console.log('posting observer lifecycle: PASS (coalescing, late body, hidden/resume/restart, authoritative context recovery, read-only fallback, stale failure and 401)');
})().catch(error=>{console.error(error);process.exitCode=1;});
