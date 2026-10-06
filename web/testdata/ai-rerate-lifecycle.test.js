'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const script = fs.readFileSync(path.join(__dirname, '..', 'ai-rerate.js'), 'utf8');
const activeCopy = 'AI로 다시 분석하는 중이에요 — 여러 공고를 한 번에 살펴보고 있어요. ☕';
const estimateCopy = 'AI로 공고를 다시 분석하고 있어요. 약 5–10분 정도 걸릴 수 있어요. 잠시 커피를 마시거나 다른 일을 하고 오셔도 좋아요. ☕ 공고 수와 AI 응답 속도에 따라 더 오래 걸릴 수 있어요.';
const completedCopy = 'AI 평가가 완료됐어요. 새로운 평가 결과를 반영했습니다.';
// Virtual-time constants mirroring the client's delay knobs: silenceWatchMs
// is how long an OPEN stream may stay silent before a status probe, and
// retryDelayMs / maxStatusRetries bound recovery fetches.
const silenceWatchMs = 15000;
const retryDelayMs = 3000;
const maxStatusRetries = 20;
const statusDeadlineMs = 10000;

// estimateRetained reports whether the estimate copy is what the page is
// showing right now (the visible status carries the 5–10분/coffee copy).
function estimateRetained(page) {
  const text = page.text('rerate-status') || '';
  return text.indexOf('5–10분') !== -1 && text.indexOf('커피') !== -1;
}

class Storage {
  constructor() { this.values = new Map(); }
  get length() { return this.values.size; }
  key(index) { return Array.from(this.values.keys())[index] || null; }
  getItem(key) { return this.values.has(key) ? this.values.get(key) : null; }
  setItem(key, value) { this.values.set(key, String(value)); }
  removeItem(key) { this.values.delete(key); }
}

class Element {
  constructor(tagName, registry) {
    this.tagName = tagName;
    this.registry = registry;
    this.children = [];
    this.listeners = new Map();
    this.parentNode = null;
    this.dataset = {};
    this.disabled = false;
    this.hidden = false;
    this._id = '';
    this._text = '';
  }
  set id(value) { this._id = value; }
  get id() { return this._id; }
  set textContent(value) {
    this._text = String(value);
    this.children = [];
    if (this.id === 'rerate-log' && value === '') {
      this.registry.delete('rerate-status');
      this.registry.delete('rerate-progress');
    }
  }
  get textContent() {
    return this._text + this.children.map((child) => child.textContent || '').join('');
  }
  appendChild(child) {
    child.parentNode = this;
    this.children.push(child);
    if (child.id) this.registry.set(child.id, child);
    return child;
  }
  removeChild(child) {
    this.children = this.children.filter((candidate) => candidate !== child);
    if (child.id) this.registry.delete(child.id);
    child.parentNode = null;
    return child;
  }
  addEventListener(name, listener) {
    if (!this.listeners.has(name)) this.listeners.set(name, []);
    this.listeners.get(name).push(listener);
  }
  dispatch(name, event = {}) {
    for (const listener of this.listeners.get(name) || []) listener(event);
  }
}

function response(status) {
  return { ok: true, status: 200, json: async () => status };
}

let tokenCounter = 0;

function makePage({ storage, state = null, navigationType = 'navigate' }) {
  const registry = new Map();
  const button = new Element('button', registry);
  button.id = 'rerate';
  button.dataset.surface = 'archive';
  const log = new Element('div', registry);
  log.id = 'rerate-log';
  const activity = new Element('span', registry);
  activity.id = 'rerate-activity';
  activity.hidden = true;
  registry.set(button.id, button);
  registry.set(log.id, log);
  registry.set(activity.id, activity);

  const windowListeners = new Map();
  const history = {
    state,
    replaceState(next) { this.state = next; }
  };
  const location = {
    reloads: 0,
    reload() { this.reloads++; }
  };
  const timers = new Map();
  let nextTimer = 1;
  // Virtual clock: the client reads Date.now() only to measure stream
  // silence, so tests can advance time deterministically. setTimeout records
  // each timer's due time so run(ms) can fire them in scheduled order.
  let clock = 0;
  const dueAt = new Map();
  const DateShim = class extends Date {
    static now() { return clock; }
  };
  const sources = [];
  const fetchQueue = [];
  const fetchCalls = [];

  class MockEventSource {
    constructor(url) {
      this.url = url;
      this.closed = false;
      this.listeners = new Map();
      sources.push(this);
    }
    addEventListener(name, listener) {
      if (!this.listeners.has(name)) this.listeners.set(name, []);
      this.listeners.get(name).push(listener);
    }
    emit(name, data) {
      for (const listener of this.listeners.get(name) || []) listener({ data: String(data) });
    }
    close() { this.closed = true; }
  }

  function fetch(url, options = {}) {
    fetchCalls.push({ url, options });
    const queued = fetchQueue.shift();
    if (!queued) return Promise.reject(new Error('no queued fetch response'));
    if (queued.kind === 'immediate') return Promise.resolve(response(queued.status));
    if (queued.kind === 'failure') return Promise.reject(new Error('network down'));
    if (queued.kind === 'deferred-body') {
      // Headers arrive, the body never does until released. The abort path
      // must reject the BODY promise too, or a deadline can only fire the
      // outer fetch promise.
      const bodyPromise = new Promise((resolveBody, rejectBody) => {
        queued.resolveBody = resolveBody;
        if (options.signal && !queued.ignoreAbort) {
          options.signal.addEventListener('abort', () => {
            const error = new Error('aborted');
            error.name = 'AbortError';
            rejectBody(error);
          }, { once: true });
        }
      });
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => bodyPromise
      });
    }
    return new Promise((resolve, reject) => {
      queued.resolve = (status) => resolve(response(status));
      queued.reject = reject;
      if (options.signal && !queued.ignoreAbort) {
        options.signal.addEventListener('abort', () => {
          const error = new Error('aborted');
          error.name = 'AbortError';
          reject(error);
        }, { once: true });
      }
    });
  }

  const document = {
    title: 'jobcron test',
    visibilityState: 'visible',
    getElementById(id) { return registry.get(id) || null; },
    createElement(tagName) { return new Element(tagName, registry); },
    createTextNode(text) { return { textContent: String(text), parentNode: null }; },
    addEventListener(name, listener) {
      if (!documentListeners.has(name)) documentListeners.set(name, []);
      documentListeners.get(name).push(listener);
    }
  };
  const documentListeners = new Map();
  function dispatchDocument(name) {
    for (const listener of documentListeners.get(name) || []) listener();
  }
  const window = {
    crypto: { randomUUID: () => `entry-token-${String(++tokenCounter).padStart(8, '0')}` },
    addEventListener(name, listener) {
      if (!windowListeners.has(name)) windowListeners.set(name, []);
      windowListeners.get(name).push(listener);
    }
  };
  const context = {
    window,
    document,
    history,
    location,
    performance: { getEntriesByType: () => [{ type: navigationType }] },
    sessionStorage: storage,
    EventSource: MockEventSource,
    AbortController,
    fetch,
    encodeURIComponent,
    JSON,
    Date: DateShim,
    Math,
    Object,
    String,
    Boolean,
    setTimeout(listener, delay = 0) {
      const id = nextTimer++;
      timers.set(id, listener);
      dueAt.set(id, clock + delay);
      return id;
    },
    clearTimeout(id) { timers.delete(id); dueAt.delete(id); }
  };
  vm.runInNewContext(script, context, { filename: 'ai-rerate.js' });

  // run(ms) advances the virtual clock by ms, firing every timer that came
  // due in that window (in due order), letting each callback schedule the
  // next — the closest deterministic analogue of real elapsed time.
  async function run(ms) {
    const deadline = clock + ms;
    while (true) {
      let fireId = null;
      let fireAt = Infinity;
      for (const [id, at] of dueAt.entries()) {
        if (at <= deadline && at < fireAt) { fireAt = at; fireId = id; }
      }
      if (fireId === null) break;
      clock = Math.max(clock, fireAt);
      const listener = timers.get(fireId);
      timers.delete(fireId);
      dueAt.delete(fireId);
      if (listener) listener();
      await flush();
    }
    clock = deadline;
  }

  return {
    button,
    history,
    location,
    sources,
    fetchCalls,
    queueStatus(status) { fetchQueue.push({ kind: 'immediate', status }); },
    queueFailure() { fetchQueue.push({ kind: 'failure' }); },
    deferStatus(ignoreAbort = false) {
      const deferred = { kind: 'deferred', resolve: null, ignoreAbort };
      fetchQueue.push(deferred);
      return deferred;
    },
    // deferBodyStatus resolves the fetch HEADERS immediately but never the
    // body read (response.json) until released — a hung body is a distinct
    // transport fault from a hung fetch and both need a deadline.
    deferBodyStatus(ignoreAbort = false) {
      const deferred = { kind: 'deferred-body', resolveBody: null, ignoreAbort };
      fetchQueue.push(deferred);
      return deferred;
    },
    run,
    setHidden() { document.visibilityState = 'hidden'; },
    becomeVisible() {
      document.visibilityState = 'visible';
      dispatchDocument('visibilitychange');
    },
    dispatchWindow(name, event = {}) {
      for (const listener of windowListeners.get(name) || []) listener(event);
    },
    click() { button.dispatch('click'); },
    inject(id, text) {
      const node = new Element('p', registry);
      node.id = id;
      node.textContent = text;
      log.appendChild(node);
    },
    text(id) { return registry.get(id)?.textContent || ''; },
    has(id) { return registry.has(id); },
    timerCount() { return timers.size; }
  };
}

async function flush() {
  await Promise.resolve();
  await Promise.resolve();
  await new Promise((resolve) => setImmediate(resolve));
}

async function main() {
  const storage = new Storage();
  storage.setItem('jobcron:rerate-owner:archive:1', 'stale-entry-owner');
  storage.setItem('jobcron:rerate-notice:archive', JSON.stringify({
    entry_token: 'legacy-entry',
    run_id: '1',
    message: 'legacy notice'
  }));
  const running = {
    run_id: 1,
    run_token: 'process-a-run-1',
    owner_entry: '',
    state: 'running',
    status: activeCopy,
    progress: '공고 2/7 분석 중...'
  };

  // The server receives ownership in the request even when navigation happens
  // before the client can receive the SSE run event.
  const initiating = makePage({ storage });
  assert.equal(storage.getItem('jobcron:rerate-owner:archive:1'), null);
  assert.equal(storage.getItem('jobcron:rerate-notice:archive'), null);
  const ownerToken = initiating.history.state.jobcronRerateEntry;
  running.owner_entry = ownerToken;
  initiating.click();
  assert.match(initiating.sources[0].url, new RegExp(`entry=${ownerToken}$`));
  initiating.dispatchWindow('pagehide');

  const restoredOwner = makePage({ storage, state: initiating.history.state, navigationType: 'back_forward' });
  restoredOwner.queueStatus(running);
  restoredOwner.dispatchWindow('pageshow');
  await flush();
  assert.equal(restoredOwner.button.disabled, true);
  assert.equal(restoredOwner.text('rerate-status'), activeCopy);
  assert.equal(restoredOwner.text('rerate-progress'), running.progress);

  // A different entry on the same surface cannot adopt the run.
  const nonOwner = makePage({ storage, navigationType: 'back_forward' });
  nonOwner.queueStatus(running);
  nonOwner.dispatchWindow('pageshow');
  await flush();
  assert.equal(nonOwner.button.disabled, false);
  assert.equal(nonOwner.has('rerate-status'), false);
  assert.equal(nonOwner.has('rerate-progress'), false);
  assert.equal(nonOwner.location.reloads, 0);

  // A status response that resolves after pagehide cannot mutate or reschedule.
  const late = makePage({ storage, state: initiating.history.state, navigationType: 'back_forward' });
  const deferred = late.deferStatus();
  late.dispatchWindow('pageshow');
  await Promise.resolve();
  late.dispatchWindow('pagehide');
  deferred.resolve(running);
  await flush();
  assert.equal(late.has('rerate-status'), false);
  assert.equal(late.has('rerate-progress'), false);
  assert.equal(late.location.reloads, 0);
  assert.equal(late.timerCount(), 0);

  // Idle restoration clears stale BFCache status and progress.
  const idle = makePage({ storage, state: initiating.history.state, navigationType: 'back_forward' });
  idle.inject('rerate-status', 'stale status');
  idle.inject('rerate-progress', 'stale progress');
  idle.queueStatus({ state: 'idle' });
  idle.dispatchWindow('pageshow');
  await flush();
  assert.equal(idle.has('rerate-status'), false);
  assert.equal(idle.has('rerate-progress'), false);

  // Failed state is shown once, then the unique run token clears it as handled.
  const failedStatus = { ...running, state: 'failed', message: '검토용 실패 상태' };
  const failed = makePage({ storage, state: initiating.history.state, navigationType: 'back_forward' });
  failed.inject('rerate-progress', 'stale progress');
  failed.queueStatus(failedStatus);
  failed.dispatchWindow('pageshow');
  await flush();
  assert.equal(failed.text('rerate-status'), failedStatus.message);
  assert.equal(failed.has('rerate-progress'), false);
  assert.equal(storage.getItem('jobcron:rerate-handled:archive'), running.run_token);

  const handled = makePage({ storage, state: initiating.history.state, navigationType: 'back_forward' });
  handled.inject('rerate-status', failedStatus.message);
  handled.inject('rerate-progress', 'stale progress');
  handled.queueStatus(failedStatus);
  handled.dispatchWindow('pageshow');
  await flush();
  assert.equal(handled.has('rerate-status'), false);
  assert.equal(handled.has('rerate-progress'), false);

  // A restarted process may reuse run_id=1, but its unique run token must not
  // inherit the prior process's handled state.
  const restartedStatus = {
    ...running,
    run_token: 'process-b-run-1',
    state: 'running',
    progress: '공고 1/2 분석 중...'
  };
  const restarted = makePage({ storage, state: initiating.history.state, navigationType: 'back_forward' });
  restarted.queueStatus(restartedStatus);
  restarted.dispatchWindow('pageshow');
  await flush();
  assert.equal(restarted.button.disabled, true);
  assert.equal(restarted.text('rerate-progress'), restartedStatus.progress);

  // A terminal snapshot cannot reload or create a notice on a non-owner entry.
  storage.removeItem('jobcron:rerate-handled:archive');
  const doneStatus = {
    ...running,
    state: 'done',
    outcome: 'changed',
    message: '공고 2개를 모두 AI로 분석했어요.'
  };
  const doneNonOwner = makePage({ storage, navigationType: 'back_forward' });
  doneNonOwner.queueStatus(doneStatus);
  doneNonOwner.dispatchWindow('pageshow');
  await flush();
  assert.equal(doneNonOwner.location.reloads, 0);
  assert.equal(storage.getItem('jobcron:rerate-notice:archive'), null);
  assert.equal(storage.getItem('jobcron:rerate-handled:archive'), null);

  // The owner reloads once, sees one completion notice, then a manual reload is quiet.
  const doneOwner = makePage({ storage, state: initiating.history.state, navigationType: 'back_forward' });
  doneOwner.queueStatus(doneStatus);
  doneOwner.dispatchWindow('pageshow');
  await flush();
  assert.equal(doneOwner.location.reloads, 1);
  assert.notEqual(storage.getItem('jobcron:rerate-notice:archive'), null);
  assert.equal(storage.getItem('jobcron:rerate-handled:archive'), running.run_token);

  const reloaded = makePage({ storage, state: initiating.history.state, navigationType: 'reload' });
  assert.equal(reloaded.text('rerate-status'), completedCopy);
  assert.equal(storage.getItem('jobcron:rerate-notice:archive'), null);
  const manualReload = makePage({ storage, state: initiating.history.state, navigationType: 'reload' });
  assert.equal(manualReload.has('rerate-status'), false);

  // Cached and partial terminal snapshots keep their server-specific outcome
  // copy when completion is discovered after navigating away.
  const cachedCopy = '이미 모든 공고가 AI로 평가됐습니다. 추가 토큰은 사용하지 않았어요.';
  const cachedStatus = {
    ...doneStatus,
    run_token: 'process-a-run-cached',
    outcome: 'cached',
    message: cachedCopy
  };
  const cachedOwner = makePage({ storage, state: initiating.history.state, navigationType: 'back_forward' });
  cachedOwner.queueStatus(cachedStatus);
  cachedOwner.dispatchWindow('pageshow');
  await flush();
  assert.equal(cachedOwner.location.reloads, 1);
  const cachedReload = makePage({ storage, state: initiating.history.state, navigationType: 'reload' });
  assert.equal(cachedReload.text('rerate-status'), cachedCopy);

  const partialCopy = '공고 2/7개를 AI로 분석했어요 — 토큰을 아끼려고 한 번에 일정 개수만 분석해요. 더 보려면 다시 눌러주세요.';
  const partialStatus = {
    ...doneStatus,
    run_token: 'process-a-run-partial',
    outcome: 'partial',
    message: partialCopy
  };
  const partialOwner = makePage({ storage, state: initiating.history.state, navigationType: 'back_forward' });
  partialOwner.queueStatus(partialStatus);
  partialOwner.dispatchWindow('pageshow');
  await flush();
  assert.equal(partialOwner.location.reloads, 1);
  const partialReload = makePage({ storage, state: initiating.history.state, navigationType: 'reload' });
  assert.equal(partialReload.text('rerate-status'), partialCopy);

  const noProgressCopy = '8개는 AI가 근거를 확인하지 못했어요. 지금 다시 눌러도 같은 결과일 수 있어요.';
  const noProgressStatus = {
    ...doneStatus,
    run_token: 'process-a-run-no-progress',
    outcome: 'no_progress',
    message: noProgressCopy
  };
  const noProgressOwner = makePage({ storage, state: initiating.history.state, navigationType: 'back_forward' });
  noProgressOwner.queueStatus(noProgressStatus);
  noProgressOwner.dispatchWindow('pageshow');
  await flush();
  assert.equal(noProgressOwner.location.reloads, 1);
  const noProgressReload = makePage({ storage, state: initiating.history.state, navigationType: 'reload' });
  assert.equal(noProgressReload.text('rerate-status'), noProgressCopy);

  const emptyCopy = '지금 화면에 분석할 공고가 없어요.';
  const emptyStatus = {
    ...doneStatus,
    run_token: 'process-a-run-empty',
    outcome: 'empty',
    message: emptyCopy
  };
  const emptyOwner = makePage({ storage, state: initiating.history.state, navigationType: 'back_forward' });
  emptyOwner.queueStatus(emptyStatus);
  emptyOwner.dispatchWindow('pageshow');
  await flush();
  assert.equal(emptyOwner.location.reloads, 1);
  const emptyReload = makePage({ storage, state: initiating.history.state, navigationType: 'reload' });
  assert.equal(emptyReload.text('rerate-status'), emptyCopy);

  // The additive run-token event preserves the visible-page SSE flow.
  const visibleStorage = new Storage();
  const visible = makePage({ storage: visibleStorage });
  visible.click();
  visible.sources[0].emit('run-token', 'process-visible-run-1');
  visible.sources[0].emit('status', activeCopy);
  visible.sources[0].emit('progress', '공고 1/2 분석 중...');
  assert.equal(visible.button.disabled, true);
  assert.equal(visible.text('rerate-progress'), '공고 1/2 분석 중...');
  visible.sources[0].emit('done', '공고 2개를 모두 AI로 분석했어요.');
  assert.equal(visible.location.reloads, 1);
  assert.equal(visibleStorage.getItem('jobcron:rerate-handled:archive'), 'process-visible-run-1');

  // --- Progress copy + transport recovery (t_942f06ba) ---

  // The click-time copy must carry the calm 5–10 minute estimate so the wait
  // is explained the moment the button is pressed.
  const estimateStorage = new Storage();
  const estimate = makePage({ storage: estimateStorage });
  assert.match(estimate.button.textContent || 'AI 평가', /^AI 평가/); // element sanity
  estimate.click();
  assert.equal(estimate.text('rerate-status'), estimateCopy);

  // A stream error on a visible tab must not strand the page on dead copy:
  // the client closes the stream (no EventSource auto-reconnect → no second
  // provider run) and recovers through the status endpoint instead.
  const dropStorage = new Storage();
  const drop = makePage({ storage: dropStorage });
  drop.click();
  drop.sources[0].emit('run-token', 'process-drop-run-1');
  drop.sources[0].emit('status', estimateCopy);
  drop.sources[0].emit('progress', '공고 1/4 분석 중...');
  drop.queueStatus({ state: 'running', run_token: 'process-drop-run-1', owner_entry: drop.history.state.jobcronRerateEntry, status: estimateCopy, progress: '공고 2/4 분석 중...' });
  drop.sources[0].emit('error', {});
  assert.equal(drop.sources[0].closed, true, 'stream must close on error (no auto-reconnect into a second run)');
  await flush();
  assert.equal(drop.text('rerate-status'), estimateCopy);
  assert.equal(drop.text('rerate-progress'), '공고 2/4 분석 중...', 'owned run progress must resume via status polling');
  assert.equal(drop.button.disabled, true, 'the run is still active — button stays disabled');
  assert.equal(drop.timerCount(), 1, 'polling must be scheduled');

  // --- Review round 1: estimate survives the REAL server status sequence ---
  // The server's mid-run statuses now re-anchor the same approximate wait, so
  // a real press must show the estimate from click through the Stage-2 status,
  // never reverting to a generic "analyzing" line. This replays the exact
  // sequence the fixed server emits (opening estimate → prep progress →
  // Stage-2 estimate status → stage-2 progress) and asserts the visible copy
  // still carries the estimate after every step.
  const liveStorage = new Storage();
  const live = makePage({ storage: liveStorage });
  live.click();
  live.sources[0].emit('run-token', 'process-live-run-1');
  live.sources[0].emit('status', estimateCopy);
  live.sources[0].emit('progress', '공고 정보 확인 0/8...');
  for (let i = 1; i <= 8; i++) {
    live.sources[0].emit('progress', `공고 정보 확인 ${i}/8...`);
  }
  live.sources[0].emit('progress', '공고 문맥 확인 0/8...');
  live.sources[0].emit('progress', '공고 문맥 확인 8/8...');
  live.sources[0].emit('status', estimateCopy); // Stage-2 status re-anchors the estimate
  live.sources[0].emit('progress', '공고 0/8 분석 중...');
  live.sources[0].emit('progress', '공고 8/8 분석 중...');
  live.sources[0].emit('done', '공고 8개를 모두 AI로 분석했어요.');
  assert.ok(estimateRetained(live), 'estimate must remain visible through the full real status sequence');
  assert.equal(live.location.reloads, 1);

  // Estimate retention through error → poll recovery with the REAL server
  // running status (the tracker now holds the estimate, not the old copy).
  const recoverStorage = new Storage();
  const recover = makePage({ storage: recoverStorage });
  recover.click();
  recover.sources[0].emit('run-token', 'process-recover-run-1');
  recover.sources[0].emit('status', estimateCopy);
  recover.queueStatus({ state: 'running', run_token: 'process-recover-run-1', owner_entry: recover.history.state.jobcronRerateEntry, status: estimateCopy, progress: '공고 3/6 분석 중...' });
  recover.sources[0].emit('error', {});
  await flush();
  assert.equal(recover.text('rerate-status'), estimateCopy, 'estimate must survive stream error + owned-run poll recovery');

  // --- Review round 1: bounded status-only recovery for a silent open stream ---
  // While the stream stays OPEN but stops delivering events (proxy stall,
  // dropped middle), the client must check the status endpoint after a
  // bounded silence window — without closing the healthy stream — and adopt
  // the owned run's progress. Ownership/generation guards unchanged.
  const silentStorage = new Storage();
  const silent = makePage({ storage: silentStorage });
  silent.click();
  silent.sources[0].emit('run-token', 'process-silent-run-1');
  silent.sources[0].emit('status', estimateCopy);
  silent.queueStatus({ state: 'running', run_token: 'process-silent-run-1', owner_entry: silent.history.state.jobcronRerateEntry, status: estimateCopy, progress: '공고 5/9 분석 중...' });
  await silent.run(silenceWatchMs); // advance past the silence window with no events
  assert.equal(silent.timerCount() >= 1, true, 'a recovery probe must be scheduled while the stream is silent');
  const silentFetched = silent.fetchCalls.some((call) => call.url.indexOf('/api/rerate/status') !== -1);
  assert.equal(silentFetched, true, 'a silent open stream must trigger a status check, not just wait');
  assert.equal(silent.text('rerate-progress'), '공고 5/9 분석 중...', 'silent-stream recovery must adopt the owned run progress');
  assert.equal(silent.button.disabled, true, 'the owned run keeps the button disabled during silent-stream recovery');
  assert.equal(silent.sources[0].closed, false, 'the still-open stream must not be closed by the silence check');
  // Terminal cleanup: done fires on the stream → transport stops, no timers.
  silent.sources[0].emit('done', '공고 9개를 모두 AI로 분석했어요.');
  assert.equal(silent.timerCount(), 0, 'terminal done must clear every recovery timer');
  assert.equal(silent.location.reloads, 1);

  // A hidden tab whose stream died silently: on return, the client re-adopts
  // the owned run via one status probe (the stream may be dead but non-null).
  const awayStorage = new Storage();
  const away = makePage({ storage: awayStorage });
  away.click();
  away.sources[0].emit('run-token', 'process-away-run-1');
  away.sources[0].emit('status', estimateCopy);
  away.setHidden(); // tab hidden while the stream silently stalls
  away.queueStatus({ state: 'running', run_token: 'process-away-run-1', owner_entry: away.history.state.jobcronRerateEntry, status: estimateCopy, progress: '공고 2/5 분석 중...' });
  away.becomeVisible();
  await flush();
  assert.equal(away.text('rerate-progress'), '공고 2/5 분석 중...', 'returning to a silently-stalled tab must re-adopt the owned run');
  assert.equal(away.button.disabled, true);

  // A transiently failing status fetch during recovery must keep retrying
  // (bounded), not strand the page: connectivity returns → the poll resumes
  // and the run completes without any user action.
  const flakyStorage = new Storage();
  const flaky = makePage({ storage: flakyStorage });
  flaky.click();
  flaky.sources[0].emit('run-token', 'process-flaky-run-1');
  flaky.sources[0].emit('status', estimateCopy);
  flaky.sources[0].emit('progress', '공고 1/3 분석 중...');
  flaky.queueFailure(); // first recovery fetch fails (network down)
  flaky.sources[0].emit('error', {});
  await flush();
  assert.equal(flaky.text('rerate-status'), estimateCopy, 'a failed recovery fetch must not strand dead copy');
  assert.equal(flaky.button.disabled, true, 'a transient fetch failure must not re-enable the button (run still active server-side)');
  assert.equal(flaky.timerCount(), 1, 'a retry must be scheduled after a failed status fetch');
  flaky.queueStatus({ state: 'done', run_token: 'process-flaky-run-1', owner_entry: flaky.history.state.jobcronRerateEntry, outcome: 'changed', message: '공고 3개를 모두 AI로 분석했어요.' });
  await flaky.run(retryDelayMs);
  assert.equal(flaky.location.reloads, 1, 'connectivity return must complete the run end-to-end with no user action');

  // Bounded exhaustion: repeated failed status reads end in an explicit
  // unresolved state (page still owned, button re-enabled, honest copy),
  // never a silent infinite retry loop.
  const deadStorage = new Storage();
  const dead = makePage({ storage: deadStorage });
  dead.click();
  dead.sources[0].emit('run-token', 'process-dead-run-1');
  dead.sources[0].emit('status', estimateCopy);
  for (let i = 0; i < maxStatusRetries + 2; i++) dead.queueFailure();
  dead.sources[0].emit('error', {});
  await flush();
  for (let i = 0; i < maxStatusRetries; i++) await dead.run(retryDelayMs);
  assert.equal(dead.timerCount(), 0, 'bounded retry must terminate, not loop forever');
  assert.equal(dead.button.disabled, false, 'exhausted recovery must re-enable the button');
  assert.match(dead.text('rerate-status'), /확인하지 못했어요/, 'exhausted recovery must leave an explicit unresolved message');

  // A foreign running run discovered through silent-stream recovery is NOT
  // adopted: no progress render, no poll loop, button stays usable.
  const foreignSilentStorage = new Storage();
  const foreignSilent = makePage({ storage: foreignSilentStorage });
  foreignSilent.click();
  foreignSilent.sources[0].emit('run-token', 'process-fsilent-run-1');
  foreignSilent.sources[0].emit('status', estimateCopy);
  foreignSilent.queueStatus({ state: 'running', run_token: 'process-fsilent-run-1', owner_entry: 'someone-else', status: estimateCopy, progress: '공고 1/9 분석 중...' });
  await foreignSilent.run(silenceWatchMs);
  assert.equal(foreignSilent.button.disabled, false, 'a foreign run must never disable this page');
  assert.equal(foreignSilent.timerCount(), 0, 'a foreign run must not start a poll loop');

  // --- Review round 1: Stage-2 initial numeric before first result ---
  // The click placeholder must be numeric immediately: the client renders the
  // server's honest 0/M the moment it arrives — before any provider call
  // completes — so the counter never sits empty between click and first row.
  const numericStorage = new Storage();
  const numeric = makePage({ storage: numericStorage });
  numeric.click();
  numeric.sources[0].emit('run-token', 'process-numeric-run-1');
  numeric.sources[0].emit('status', estimateCopy);
  numeric.sources[0].emit('progress', '공고 정보 확인 0/6...');
  for (let i = 1; i <= 6; i++) {
    numeric.sources[0].emit('progress', `공고 정보 확인 ${i}/6...`);
  }
  numeric.sources[0].emit('status', estimateCopy);
  numeric.sources[0].emit('progress', '공고 0/6 분석 중...');
  assert.equal(numeric.text('rerate-progress'), '공고 0/6 분석 중...', 'the honest 0/M must render before any row completes');
  assert.equal(numeric.button.disabled, true);
  numeric.sources[0].emit('done', '공고 6개를 모두 AI로 분석했어요.');
  assert.equal(numeric.location.reloads, 1);


  // A page shown WITHOUT a history return (e.g. reload while the owned run is
  // still active on the detached server side) must adopt the owned run.
  const reloadStorage = new Storage();
  const reloadPage = makePage({ storage: reloadStorage });
  const reloadEntry = reloadPage.history.state.jobcronRerateEntry;
  reloadPage.queueStatus({ state: 'running', run_token: 'process-reload-run-1', owner_entry: reloadEntry, status: estimateCopy, progress: '공고 3/9 분석 중...' });
  reloadPage.dispatchWindow('pageshow', { persisted: false });
  await flush();
  assert.equal(reloadPage.button.disabled, true);
  assert.equal(reloadPage.text('rerate-progress'), '공고 3/9 분석 중...');

  // A non-owner running run on a fresh page must not be adopted.
  const foreignStorage = new Storage();
  const foreign = makePage({ storage: foreignStorage, navigationType: 'reload' });
  foreign.queueStatus({ state: 'running', run_token: 'process-foreign-run-1', owner_entry: 'someone-else-entry', status: estimateCopy, progress: '공고 1/9 분석 중...' });
  foreign.dispatchWindow('pageshow', { persisted: false });
  await flush();
  assert.equal(foreign.button.disabled, false);
  assert.equal(foreign.has('rerate-progress'), false);

  // The completion notice shown right after the completion reload must SURVIVE
  // the pageshow status poll: done+handled must not clobber the fresh notice.
  const noticeStorage = new Storage();
  const noticeEntry = 'entry-token-notice001';
  noticeStorage.setItem('jobcron:rerate-notice:archive', JSON.stringify({
    entry_token: noticeEntry,
    run_token: 'process-notice-run-1',
    message: completedCopy
  }));
  noticeStorage.setItem('jobcron:rerate-handled:archive', 'process-notice-run-1');
  const noticeState = {};
  noticeState.jobcronRerateEntry = noticeEntry;
  const noticePage = makePage({ storage: noticeStorage, state: noticeState, navigationType: 'reload' });
  noticePage.queueStatus({ state: 'done', run_token: 'process-notice-run-1', owner_entry: noticeEntry, outcome: 'changed', message: completedCopy });
  noticePage.dispatchWindow('pageshow', { persisted: false });
  await flush();
  assert.equal(noticePage.text('rerate-status'), completedCopy, 'fresh completion notice must survive the done+handled poll');
  assert.equal(noticePage.has('rerate-progress'), false);

  // --- Review round 2 (run104): deadlined, ordered, identity-fenced recovery ---

  // F1a: an unresolved status FETCH after stream loss must not strand the
  // page forever — the fetch gets a deadline; expiry enters the SAME bounded
  // retry/exhaustion path as a rejection (never an infinite hang).
  const hungStorage = new Storage();
  const hung = makePage({ storage: hungStorage });
  hung.click();
  hung.sources[0].emit('run-token', 'process-hung-run-1');
  hung.sources[0].emit('status', estimateCopy);
  hung.deferStatus(); // stream errors → recovery fetch that never resolves
  hung.sources[0].emit('error', {});
  await flush();
  // One full silence window + deadline + retry delay: the deadlined fetch
  // must have expired and been replaced by a fresh attempt.
  await hung.run(silenceWatchMs + statusDeadlineMs + retryDelayMs);
  assert.ok(hung.fetchCalls.length >= 2, `a deadlined fetch must retry, not hang (fetches=${hung.fetchCalls.length})`);
  assert.ok(hung.timerCount() >= 1, 'deadline expiry must schedule the next bounded retry');
  assert.equal(hung.button.disabled, true, 'the owned run keeps the button disabled through retries');

  // F1b: a resolved fetch whose BODY never arrives is the same fault — the
  // deadline must cover the body read, not only the fetch.
  const hungBodyStorage = new Storage();
  const hungBody = makePage({ storage: hungBodyStorage });
  hungBody.click();
  hungBody.sources[0].emit('run-token', 'process-hungbody-run-1');
  hungBody.sources[0].emit('status', estimateCopy);
  hungBody.deferBodyStatus();
  hungBody.sources[0].emit('error', {});
  await flush();
  await hungBody.run(silenceWatchMs + statusDeadlineMs + retryDelayMs);
  assert.ok(hungBody.fetchCalls.length >= 2, `a deadlined body read must retry, not hang (fetches=${hungBody.fetchCalls.length})`);
  assert.ok(hungBody.timerCount() >= 1, 'body-deadline expiry must schedule the next bounded retry');

  // F1c: repeated deadlined reads end in the SAME explicit unresolved state
  // as rejections — never an unbounded fetch loop.
  const deadDeadlineStorage = new Storage();
  const deadDeadline = makePage({ storage: deadDeadlineStorage });
  deadDeadline.click();
  deadDeadline.sources[0].emit('run-token', 'process-deadline-run-1');
  deadDeadline.sources[0].emit('status', estimateCopy);
  for (let i = 0; i < maxStatusRetries + 2; i++) deadDeadline.deferStatus();
  deadDeadline.sources[0].emit('error', {});
  await flush();
  for (let i = 0; i < maxStatusRetries; i++) await deadDeadline.run(statusDeadlineMs + retryDelayMs);
  assert.equal(deadDeadline.timerCount(), 0, 'deadlined retries must exhaust within the bounded budget');
  assert.equal(deadDeadline.button.disabled, false, 'exhausted deadlined recovery must re-enable the button');
  assert.match(deadDeadline.text('rerate-status'), /확인하지 못했어요/, 'deadlined exhaustion must leave the explicit unresolved message');

  // F2a: a LATE status response must not overwrite newer stream progress.
  // The stream is live at 3/4 when an older 0/4 snapshot finally resolves —
  // the stale adoption is dropped (fenced by observation order).
  const staleStorage = new Storage();
  const stale = makePage({ storage: staleStorage });
  stale.click();
  stale.sources[0].emit('run-token', 'process-stale-run-1');
  stale.sources[0].emit('status', estimateCopy);
  const staleDeferred = stale.deferStatus();
  await stale.run(silenceWatchMs); // watchdog probe fires, stays pending
  stale.sources[0].emit('progress', '공고 3/4 분석 중...'); // stream moves on
  staleDeferred.resolve({ state: 'running', run_token: 'process-stale-run-1', owner_entry: stale.history.state.jobcronRerateEntry, status: estimateCopy, progress: '공고 0/4 분석 중...' });
  await flush();
  assert.equal(stale.text('rerate-progress'), '공고 3/4 분석 중...', 'a late status response must not regress newer stream progress');
  assert.equal(stale.sources[0].closed, false, 'the live stream stays open');

  // F2b: terminal adoption must invalidate outstanding probes and the still
  // open stream — a late RUNNING response after a terminal outcome can never
  // resurrect loading state or re-arm timers.
  const terminalStorage = new Storage();
  const terminal = makePage({ storage: terminalStorage });
  terminal.click();
  terminal.sources[0].emit('run-token', 'process-terminal-run-1');
  terminal.sources[0].emit('status', estimateCopy);
  const terminalDeferred = terminal.deferStatus();
  await terminal.run(silenceWatchMs); // watchdog probe pending
  terminal.becomeVisible(); // coalesced; cannot start a second competing read
  assert.equal(terminal.fetchCalls.length, 1);
  terminal.sources[0].emit('failed', 'synthetic terminal failure');
  await flush();
  assert.equal(terminal.button.disabled, false, 'terminal adoption must end loading');
  assert.equal(terminal.text('rerate-status'), 'synthetic terminal failure');
  assert.equal(terminal.timerCount(), 0, 'terminal adoption must clear every recovery timer');
  assert.equal(terminal.sources[0].closed, true, 'terminal adoption must close the still-open stream');
  terminalDeferred.resolve({ state: 'running', run_token: 'process-terminal-run-1', owner_entry: terminal.history.state.jobcronRerateEntry, status: estimateCopy, progress: '공고 1/4 분석 중...' });
  await flush();
  assert.equal(terminal.button.disabled, false, 'a late running response must not resurrect loading');
  assert.equal(terminal.timerCount(), 0, 'a late running response must not re-arm timers');
  assert.equal(terminal.text('rerate-status'), 'synthetic terminal failure', 'a late running response must not clear the terminal notice');

  // F2c: identity fence — the client knows the run_token of the run it
  // started; a status response for a DIFFERENT run (same owner entry, e.g.
  // another tab of the same entry after a reload started a newer run) must
  // never overwrite the known active run's progress. Only a terminal state of
  // a foreign run may stop the local loop (the known run is gone server-side).
  const mismatchStorage = new Storage();
  const mismatch = makePage({ storage: mismatchStorage });
  mismatch.click();
  mismatch.sources[0].emit('run-token', 'process-known-run-1');
  mismatch.sources[0].emit('status', estimateCopy);
  mismatch.sources[0].emit('progress', '공고 2/5 분석 중...');
  mismatch.queueStatus({ state: 'running', run_token: 'process-other-run-9', owner_entry: mismatch.history.state.jobcronRerateEntry, status: estimateCopy, progress: '공고 9/9 분석 중...' });
  await mismatch.run(silenceWatchMs); // watchdog probe returns the foreign run
  assert.equal(mismatch.text('rerate-progress'), '공고 2/5 분석 중...', 'a foreign running run must not overwrite the known active run progress');
  assert.equal(mismatch.button.disabled, false, 'known-token mismatch must boundedly end loading');
  assert.equal(mismatch.sources[0].closed, true, 'mismatch must settle the source too');
  assert.equal(mismatch.timerCount(), 0, 'a foreign running run must not start a poll loop');

  // F2c-2: same identity fence with the stream DEAD (recovery poll owns the
  // loop): the known run is gone server-side — the loop must end terminally
  // safe (no strand, no foreign-run polling), with an explicit hint.
  const goneStorage = new Storage();
  const gone = makePage({ storage: goneStorage });
  gone.click();
  gone.sources[0].emit('run-token', 'process-gone-run-1');
  gone.sources[0].emit('status', estimateCopy);
  gone.sources[0].emit('progress', '공고 2/5 분석 중...');
  gone.queueStatus({ state: 'running', run_token: 'process-newer-run-2', owner_entry: gone.history.state.jobcronRerateEntry, status: estimateCopy, progress: '공고 1/2 분석 중...' });
  gone.sources[0].emit('error', {}); // stream dies → full poll adopts the foreign run
  await flush();
  await gone.run(retryDelayMs); // let one scheduled poll cycle observe cleanup
  assert.equal(gone.button.disabled, false, 'a foreign running run during dead-stream recovery must end loading');
  assert.equal(gone.timerCount(), 0, 'the dead-stream recovery loop must stop on a foreign run');
  assert.equal(gone.text('rerate-progress'), '공고 2/5 분석 중...', 'the known run progress stays honest on screen');
  assert.match(gone.text('rerate-status'), /종료됐어요/, 'the user is told the known run ended and how to see the newest state');

}

module.exports = { makePage, Storage, flush, estimateCopy };
if (require.main === module) {
  main().catch((error) => {
    console.error(error.stack || error);
    process.exitCode = 1;
  });
}
