'use strict';
const assert=require('node:assert/strict');const fs=require('node:fs');const path=require('node:path');const vm=require('node:vm');
const flush=async()=>{for(let i=0;i<12;i++)await Promise.resolve();};
(async()=>{
 for(const script of ['bookmark.js','not-interested.js'])for(const fails of [false,true]) {
  let click,resolve,reject,resyncs=0,on=false;
  const classes={contains:()=>on,toggle:(name,value)=>{on=value===undefined?!on:!!value;},add(){}};
  const card={classList:{add(){}},addEventListener(){},remove(){throw Error('late reply removed a newer stored card');}};
  const button={classList:classes,disabled:false,dataset:{postingId:'1'},setAttribute(){},closest:()=>card};
  const context={document:{body:{dataset:{demo:'false'}},addEventListener:(name,fn)=>{if(name==='click')click=fn;},readyState:'complete',querySelector:()=>null,querySelectorAll:()=>[],dispatchEvent(){}},
   window:{JobcronPostingUpdates:{resync(){resyncs++;}}},location:{pathname:'/bookmarks'},fetch:()=>new Promise((a,b)=>{resolve=a;reject=b;}),setTimeout(){},CustomEvent:class{},console};
  vm.runInNewContext(fs.readFileSync(path.join(__dirname,'..',script),'utf8'),context);
  click({target:{closest:()=>button}});assert.equal(button.disabled,true);
  // A newer authoritative snapshot has applied while the mutation body waits.
  on=true;
  if(fails)reject(Error('synthetic transport failure'));else resolve({ok:true,status:200,json:async()=>({bookmarked:false,not_interested:false})});
  await flush();assert.equal(on,true,script+' late response undid a newer snapshot');assert.equal(button.disabled,false);assert.equal(resyncs,1,'settlement must read stored state, not replay a mutation');
 }
 console.log('live mutation settlement: PASS (pending controls, late success/failure, read-only reconciliation, no replay)');
})().catch(error=>{console.error(error);process.exitCode=1;});
