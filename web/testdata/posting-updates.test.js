'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const box = {module:{exports:{}},document:{readyState:'loading',addEventListener(){}}};
vm.runInNewContext(fs.readFileSync(path.join(__dirname,'../posting-updates.js'),'utf8'),box);
const admit=box.module.exports.viewAdmission;
const last={epoch:'epoch-a',global_revision:'2',user_revision:'4',context:'context-a'};
const request={generation:'7',surface:'archive',sort:'date',owner:1,contextSequence:0,force:false};
const current={owner:1,contextSequence:0,epoch:'epoch-a',context:'context-a',user_revision:'4',retired:[]};
const view={schema:'posting-view-v1',epoch:'epoch-a',global_revision:'3',user_revision:'5',context:'context-a',generation:'7',surface:'archive',sort:'date',rows:[],sections:[],total:0,main:0,excluded:0};
assert.equal(admit(view,request,last,{...current,user_revision:'999'}),true,'a newer dirty hint must not starve coherent snapshots newer than last APPLIED');
for (const changes of [{schema:'other'},{generation:'6'},{surface:'today'},{sort:'score'},{global_revision:'1'},{user_revision:'3'},{context:'old-context'}]) {
 assert.equal(admit({...view,...changes},request,last,current),false,JSON.stringify(changes));
}
assert.equal(admit(view,request,last,{...current,owner:2}),false,'late lifecycle body');
assert.equal(admit(view,request,last,{...current,contextSequence:1,context:'context-b'}),false,'context change invalidates old body');
assert.equal(admit({...view,context:'context-b',user_revision:'8'},{...request,contextSequence:1},last,{...current,contextSequence:1,context:'context-b',user_revision:'8'}),true,'new-context read');
assert.equal(admit({...view,epoch:'epoch-b'},request,last,current),false,'old ordinary read cannot declare a restart');
assert.equal(admit({...view,epoch:'epoch-b'},{...request,force:true},last,{...current,epoch:''}),true,'forced stored-state recovery admits new process epoch');
assert.equal(admit(view,{...request,force:true},null,{...current,retired:['epoch-a']}),false,'retired epoch never returns');
assert.equal(admit({...view,context:'old-context'},{...request,force:true},last,{...current,contextLocked:true}),false,'forced resync cannot bypass the latest context notice');
assert.equal(admit({...view,context:'external-context',user_revision:'5'},{...request,force:true,baseline:true},last,{...current,contextLocked:true,contextRevision:'6'}),false,'baseline cannot admit a response below the announced commit');
assert.equal(admit({...view,context:'external-context',user_revision:'6'},{...request,force:true,baseline:true},last,{...current,contextLocked:true,contextRevision:'6'}),true,'post-notice baseline may recover superseding external state');
assert.equal(admit({...view,user_revision:'bad'},request,last,current),false,'closed revision grammar');
console.log('posting snapshot admission: PASS (ordinary-write convergence, context/epoch/lifecycle/schema/surface/sort/generation and numeric gates)');
