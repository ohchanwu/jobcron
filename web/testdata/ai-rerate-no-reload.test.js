'use strict';
const assert=require('node:assert/strict');
const {makePage,Storage}=require('./ai-rerate-lifecycle.test.js');
for(const event of ['done','failed']) {
 const p=makePage({storage:new Storage()});p.click();p.sources[0].emit('run-token','native-owned-run');p.sources[0].emit(event,'synthetic terminal');
 assert.equal(p.location.reloads,0,'terminal never reloads');assert.equal(p.location.snapshots,1,'terminal requests current stored state');
 assert.equal(p.sources.length,1,'no paid restart');assert.equal(p.sources[0].closed,true);
}
console.log('owned operation terminal: PASS (no reload, one stored-state reconciliation, no paid restart)');
