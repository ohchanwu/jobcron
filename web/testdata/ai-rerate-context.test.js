'use strict';
const assert=require('node:assert/strict');
const {makePage,Storage,flush}=require('./ai-rerate-lifecycle.test.js');
(async()=>{
 for(const surface of ['today','bookmarks','archive']){
  const p=makePage({storage:new Storage(),enabled:false,surface});
  p.click();assert.equal(p.sources.length,0,'AI-off shell cannot start work');
  p.applyView({epoch:'e1',context:'on',surface,rerate:{Surface:surface,Visible:0,Analyzed:0}});
  assert.equal(p.button.disabled,false,'live AI-on initializes deliberate control');
  assert.equal(p.sources.length,0,'enabling is not an operation');
  p.applyView({epoch:'e1',context:'on',surface,rerate:{Surface:surface,Visible:7,Analyzed:5}});
  p.click();assert.equal(p.sources.length,1,'one handler across repeated projections');
  assert.match(p.sources[0].url,new RegExp('surface='+surface));
  p.sources[0].emit('run-token','old-run');
  const late=p.deferBodyStatus(true);await p.run(15000);
  p.applyView({epoch:'e1',context:'off',surface,rerate:null});
  assert.equal(p.sources[0].closed,true);assert.equal(p.button.disabled,true);
  late.resolveBody({state:'done',run_token:'old-run',owner_entry:p.history.state.jobcronRerateEntry,outcome:'partial',message:'OLD CONTEXT'});
  p.sources[0].emit('failed','OLD SOURCE');await flush();
  assert.equal(p.text('rerate-status'),'');assert.equal(p.location.snapshots,0,'old owner cannot resync or replay');
  p.click();assert.equal(p.sources.length,1);
  p.applyView({epoch:'e2',context:'on-again',surface,rerate:{Surface:surface,Visible:7,Analyzed:5}});
  p.click();assert.equal(p.sources.length,2,'one fresh deliberate operation after restart/on');
  p.applyView({epoch:'e2',context:'on-again',surface,rerate:{Surface:surface,Visible:7,Analyzed:6}});
  assert.equal(p.button.disabled,true,'same-context counts retain in-flight owner');
  p.sources[1].emit('run-token','new-run');p.sources[1].emit('done','NEW RESULT');
  assert.equal(p.text('rerate-status'),'NEW RESULT');assert.equal(p.location.snapshots,1);
 }
 console.log('AI control context lifecycle: PASS (all surfaces, off/on/off, no auto call, one handler and fenced late owners)');
})().catch(error=>{console.error(error);process.exitCode=1;});
