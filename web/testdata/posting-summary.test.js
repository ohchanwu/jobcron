'use strict';
const assert=require('node:assert/strict');
const {createRunSummary}=require('../posting-updates.js');
class Node {
 constructor(tag){this.tagName=tag;this.children=[];this.text='';this.listeners={};}
 set textContent(value){this.text=String(value);this.children=[];}get textContent(){return this.text+this.children.map(n=>n.textContent).join('');}
 appendChild(node){this.children.push(node);return node;}addEventListener(name,fn){this.listeners[name]=fn;}click(){this.listeners.click();}
}
const node=new Node('div'),doc={createElement:tag=>new Node(tag)};
const update=createRunSummary(doc,node);
function view(runs,epoch='epoch-a'){return {epoch,runs};}
function run(id,state='done',outcome='partial',message='분석 결과를 저장하지 못했어요 — 저장된 결과만 보여드릴게요.'){return {run_id:id,run_token:'tracker-'+id,state,outcome,message,owner_entry:'OTHER TAB'};}
update(view({today:run(1)}));assert.match(node.textContent,/저장하지 못했어요/,'coalesced scheduled/noop terminal is visible on any surface');
node.children[0].children[1].click();assert.equal(node.textContent,'','deliberate dismissal');
update(view({today:run(1)}));assert.equal(node.textContent,'','same scoped identity remains dismissed');
update(view({today:run(2,'running','', '' )}));assert.match(node.textContent,/분석하는 중/,'new run replaces dismissed warning');
update(view({today:run(2),bookmarks:run(3,'failed','', '일부 결과를 확인하지 못했어요.')}));assert.match(node.textContent,/저장하지/);assert.match(node.textContent,/일부 결과/);
update(view({today:run(1)}));assert.equal(node.textContent,'','old run cannot replace current identity');
update(view({archive:run(4,'done','changed','OWNER COMPLETION')}));assert.equal(node.textContent,'','successful initiating-owner notice is not replayed');
update(view({today:run(1)},'epoch-b'));assert.match(node.textContent,/저장하지/,'new process resets dismissal/identity without numeric comparison');
update(view({},'epoch-c'));assert.equal(node.textContent,'','restart unknown/idle clears transient summaries');
console.log('scoped run summaries: PASS (coalesced terminal, dismissal, replacement, cross-surface, no owner replay and restart)');
