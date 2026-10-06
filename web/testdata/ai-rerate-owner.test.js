'use strict';
const assert = require('node:assert/strict');
const { makePage, Storage, flush, estimateCopy } = require('./ai-rerate-lifecycle.test.js');
const tests = [];
function test(name, fn) { tests.push({ name, fn }); }
function start() {
  const p = makePage({ storage: new Storage() });
  p.click();
  p.sources[0].emit('run-token', 'known-run');
  p.sources[0].emit('progress', '공고 2/4 분석 중...');
  return p;
}
function snapshot(p, extra = {}) {
  return { state: 'running', run_token: 'known-run', owner_entry: p.history.state.jobcronRerateEntry,
    status: estimateCopy, progress: '공고 2/4 분석 중...', ...extra };
}
function settled(p) {
  assert.equal(p.button.disabled, false);
  assert.equal(p.sources[0].closed, true);
  assert.equal(p.timerCount(), 0);
}

test('watchdog and visibility coalesce one in-flight observation', async () => {
  const p = start();
  const pending = p.deferStatus();
  await p.run(15000);
  p.becomeVisible(); p.becomeVisible();
  await flush();
  assert.equal(p.fetchCalls.length, 1, 'concurrent triggers must not start a second status read');
  pending.resolve(snapshot(p, { state: 'failed', message: 'owned failure' }));
  await flush();
  settled(p);
  assert.equal(p.text('rerate-status'), 'owned failure');
});

test('closed source callbacks cannot erase recovery or terminate it', async () => {
  const p = start();
  const pending = p.deferStatus();
  p.sources[0].emit('error', {});
  await flush();
  p.sources[0].emit('progress', 'CLOSED SOURCE');
  p.sources[0].emit('done', 'CLOSED SOURCE DONE');
  assert.equal(p.location.reloads, 0);
  assert.equal(p.text('rerate-progress'), '공고 2/4 분석 중...');
  pending.resolve(snapshot(p, { state: 'done', outcome: 'partial', message: 'owned partial' }));
  await flush();
  assert.equal(p.location.reloads, 1);
});

// The old concurrent foreign/late schedules are now unreachable. Keep the
// equivalent late fetch AND body schedules after timeout -> foreign settlement,
// with deliberately non-cooperative promises so abort alone cannot pass them.
for (const body of [false, true]) {
  test('foreign settlement ignores genuinely late ' + (body ? 'body' : 'fetch'), async () => {
    const p = start();
    const old = body ? p.deferBodyStatus(true) : p.deferStatus(true);
    await p.run(15000);
    p.becomeVisible();
    assert.equal(p.fetchCalls.length, 1);
    await p.run(10000);
    p.queueStatus(snapshot(p, { owner_entry: 'foreign-entry' }));
    await p.run(3000);
    settled(p);
    const message = p.text('rerate-status');
    if (body) old.resolveBody(snapshot(p)); else old.resolve(snapshot(p));
    await flush();
    settled(p);
    assert.equal(p.text('rerate-status'), message);
    assert.equal(p.location.reloads, 0);
  });
}

test('exhaustion closes source and ignores every late source callback', async () => {
  const p = start();
  for (let i = 0; i < 20; i++) p.queueFailure();
  await p.run(15000 + 20 * 3000);
  settled(p);
  assert.equal(p.fetchCalls.length, 20);
  const message = p.text('rerate-status');
  assert.match(message, /확인하지 못했어요/);
  for (const event of ['error', 'run-token', 'status', 'progress', 'done', 'failed']) {
    p.sources[0].emit(event, 'LATE SOURCE');
  }
  p.becomeVisible(); p.dispatchWindow('pageshow');
  await flush();
  settled(p);
  assert.equal(p.fetchCalls.length, 20);
  assert.equal(p.location.reloads, 0);
  assert.equal(p.text('rerate-status'), message);
  // A deliberate new click is a new owner, not a recovery restart.
  p.click();
  p.sources[1].emit('run-token', 'next-run');
  p.sources[0].emit('failed', 'OLD FAILURE');
  p.sources[1].emit('progress', '공고 0/3 분석 중...');
  assert.equal(p.sources.length, 2);
  assert.equal(p.button.disabled, true);
  assert.equal(p.text('rerate-progress'), '공고 0/3 분석 중...');
  p.sources[1].emit('done', 'next complete');
  assert.equal(p.location.reloads, 1);
});

for (const state of ['running', 'done', 'failed', 'idle']) {
  test('known identity is fenced before ' + state + ' effects', async () => {
    const p = start();
    p.queueStatus(snapshot(p, { state, run_token: 'different-run', message: 'OTHER RUN', outcome: 'changed' }));
    await p.run(15000);
    settled(p);
    assert.equal(p.location.reloads, 0);
    assert.notEqual(p.text('rerate-status'), 'OTHER RUN');
    assert.notEqual(p.text('rerate-progress'), '공고 9/9 분석 중...');
    assert.match(p.text('rerate-status'), /종료됐어요/, 'even idle with a mismatched known token must pass identity admission');
  });
}

test('safe reload adoption pins the first token including terminal observations', async () => {
  for (const state of ['running', 'done', 'failed']) {
    const p = makePage({ storage: new Storage() });
    p.queueStatus(snapshot(p));
    p.dispatchWindow('pageshow');
    await flush();
    assert.equal(p.text('rerate-progress'), '공고 2/4 분석 중...');
    p.queueStatus(snapshot(p, { state, run_token: 'different-run', progress: '공고 9/9 분석 중...', message: 'OTHER RUN' }));
    await p.run(750);
    assert.equal(p.button.disabled, false);
    assert.equal(p.timerCount(), 0);
    assert.equal(p.location.reloads, 0);
    assert.equal(p.text('rerate-progress'), '공고 2/4 분석 중...');
    assert.notEqual(p.text('rerate-status'), 'OTHER RUN');
  }
});

for (const error of [false, true]) {
  test('new SSE invalidates late status ' + (error ? 'error' : 'success'), async () => {
    const p = start();
    const old = p.deferStatus(true);
    await p.run(15000);
    p.sources[0].emit('progress', '공고 3/4 분석 중...');
    if (error) old.reject(new Error('stale transport error'));
    else old.resolve(snapshot(p, { progress: '공고 0/4 분석 중...' }));
    await flush();
    assert.equal(p.text('rerate-progress'), '공고 3/4 분석 중...');
    assert.equal(p.timerCount(), 1);
    await p.run(3000);
    assert.equal(p.fetchCalls.length, 1, 'stale error must not schedule retry or count a failure');
  });
}

test('one hung body deadline counts one failure despite coalesced triggers and late rejection', async () => {
  const p = start();
  p.deferBodyStatus();
  await p.run(15000);
  for (let i = 0; i < 20; i++) p.becomeVisible();
  await p.run(10000);
  for (let i = 0; i < 18; i++) p.queueFailure();
  await p.run(18 * 3000);
  assert.equal(p.fetchCalls.length, 19);
  assert.equal(p.button.disabled, true, 'timeout and abort rejection must not double count');
  p.queueFailure();
  await p.run(3000);
  settled(p);
  assert.equal(p.fetchCalls.length, 20);
});

test('hidden error resumes on visibility and history return preserves identity ownership', async () => {
  const p = start();
  p.setHidden();
  p.sources[0].emit('error', {});
  assert.equal(p.sources[0].closed, true);
  assert.equal(p.fetchCalls.length, 0);
  p.queueStatus(snapshot(p));
  p.becomeVisible();
  await flush();
  assert.equal(p.button.disabled, true);
  p.dispatchWindow('pagehide');
  p.queueStatus(snapshot(p, { state: 'done', outcome: 'partial', message: 'partial on history return' }));
  p.dispatchWindow('pageshow', { persisted: true });
  await flush();
  assert.equal(p.location.reloads, 1);
  assert.equal(p.sources.length, 1, 'history recovery must never start provider stream');
});

(async () => {
  for (const { name, fn } of tests) {
    await fn();
    console.log('PASS ' + name);
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
