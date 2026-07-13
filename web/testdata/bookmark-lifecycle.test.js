'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const bookmarkScript = fs.readFileSync(path.join(__dirname, '..', 'bookmark.js'), 'utf8');
const filterScript = fs.readFileSync(path.join(__dirname, '..', 'source-filter.js'), 'utf8');

class ClassList {
  constructor(names = []) { this.names = new Set(names); }
  add(name) { this.names.add(name); }
  contains(name) { return this.names.has(name); }
  toggle(name, force) {
    const on = force === undefined ? !this.names.has(name) : Boolean(force);
    if (on) this.names.add(name); else this.names.delete(name);
    return on;
  }
}

function makeHarness(options = {}) {
  const route = options.route || '/bookmarks';
  const demo = Boolean(options.demo);
  const sources = options.sources || ['jumpit'];
  const titles = options.titles || sources.map((_, index) => '공고 ' + (index + 1));
  const response = options.response || { ok: true, bookmarked: false };
  const documentListeners = new Map();
  const timers = [];
  const storage = new Map();
  if (demo) storage.set('jobcronDemoBookmarks', JSON.stringify(sources.map((_, index) => String(index + 1))));
  if (options.filterSource) storage.set('sourceFilter', options.filterSource);

  function listenersFor(target, type) {
    if (!target.listeners.has(type)) target.listeners.set(type, []);
    return target.listeners.get(type);
  }

  const count = { textContent: String(sources.length) };
  const list = { hidden: false };
  const pageEmpty = { hidden: true };
  const filterEmpty = { hidden: true, textContent: '' };
  const searchInput = {
    value: options.searchValue || '',
    listeners: new Map(),
    addEventListener(type, fn) { listenersFor(this, type).push(fn); },
  };

  const pills = [
    { dataset: { source: '_all' }, textContent: '전체', classList: new ClassList(['source-pill', 'on']), attributes: {} },
    ...Array.from(new Set(sources)).map((source) => ({
      dataset: { source },
      textContent: source,
      classList: new ClassList(['source-pill']),
      attributes: {},
    })),
  ];
  for (const pill of pills) {
    pill.setAttribute = (name, value) => { pill.attributes[name] = value; };
  }

  const sourceContainer = {
    dataset: { emptyTemplate: '저장한 {label} 공고가 없어요.' },
    listeners: new Map(),
    addEventListener(type, fn) { listenersFor(this, type).push(fn); },
    querySelectorAll(selector) { return selector === '.source-pill' ? pills : []; },
    querySelector(selector) {
      if (selector === '.source-pill.on') {
        return pills.find((pill) => pill.classList.contains('on')) || null;
      }
      const match = selector.match(/data-source="([^"]+)"/);
      return match ? pills.find((pill) => pill.dataset.source === match[1]) || null : null;
    },
  };

  let buttons;
  const cards = sources.map((source, index) => ({
    dataset: { source },
    hidden: false,
    isConnected: true,
    classList: new ClassList(['posting']),
    listeners: new Map(),
    addEventListener(type, fn) { listenersFor(this, type).push(fn); },
    emit(type) {
      for (const fn of this.listeners.get(type) || []) fn({ target: this });
    },
    remove() { this.isConnected = false; },
    closest(selector) { return selector === '.posting' ? this : null; },
    querySelector(selector) {
      if (selector === '.bookmark[data-posting-id]') return buttons[index];
      if (selector === '.posting-title') return { textContent: titles[index] };
      if (selector === '.posting-meta span') return { textContent: '회사 ' + (index + 1) };
      return null;
    },
  }));

  buttons = cards.map((card, index) => ({
    dataset: { postingId: String(index + 1) },
    disabled: false,
    classList: new ClassList(['bookmark', 'on']),
    attributes: { 'aria-pressed': 'true' },
    setAttribute(name, value) { this.attributes[name] = value; },
    closest(selector) {
      if (selector === '.bookmark') return this;
      if (selector === '.posting') return card;
      return null;
    },
  }));

  function liveCards() { return cards.filter((card) => card.isConnected); }
  function addDocumentListener(type, fn) {
    if (!documentListeners.has(type)) documentListeners.set(type, []);
    documentListeners.get(type).push(fn);
  }
  function emitDocument(type, event = {}) {
    for (const fn of documentListeners.get(type) || []) fn(event);
  }

  const document = {
    readyState: 'complete',
    body: { dataset: { demo: String(demo) } },
    addEventListener: addDocumentListener,
    dispatchEvent(event) { emitDocument(event.type, event); },
    emit: emitDocument,
    getElementById(id) {
      if (id === 'source-filter') return sourceContainer;
      if (id === 'source-filter-empty') return filterEmpty;
      if (id === 'posting-search') return searchInput;
      return null;
    },
    querySelector(selector) {
      if (selector === 'meta[name="csrf-token"]') return { getAttribute: () => 'csrf-test' };
      if (selector === '.count strong') return count;
      if (selector === '.empty') return pageEmpty;
      if (selector === '[data-bookmarks-empty]') return demo ? null : pageEmpty;
      if (selector === '.postings' || selector === 'ol.postings') return list;
      if (selector === '.excluded-box') return null;
      return null;
    },
    querySelectorAll(selector) {
      if (selector === '.bookmark[data-posting-id]') return buttons;
      if (selector === '.posting[data-source]') return cards;
      if (selector === '.posting') return liveCards();
      if (selector === '.posting:not([hidden])') return liveCards().filter((card) => !card.hidden);
      if (selector === '.archive-day') return [];
      return [];
    },
  };

  const context = {
    CSS: { escape: (value) => value },
    CustomEvent: function CustomEvent(type) { this.type = type; },
    document,
    fetch: async () => {
      if (response.reject) throw new Error('network failure');
      return {
        ok: response.ok,
        status: response.status || (response.ok ? 200 : 500),
        json: async () => ({ bookmarked: response.bookmarked }),
      };
    },
    localStorage: {
      getItem: (key) => storage.has(key) ? storage.get(key) : null,
      setItem: (key, value) => storage.set(key, String(value)),
    },
    location: { pathname: route },
    setTimeout: (fn) => { timers.push(fn); return timers.length; },
    window: { CSS: { escape: (value) => value } },
  };
  vm.runInNewContext(bookmarkScript, context, { filename: 'bookmark.js' });
  vm.runInNewContext(filterScript, context, { filename: 'source-filter.js' });

  return { buttons, cards, count, document, filterEmpty, list, pageEmpty, storage, timers };
}

async function click(harness, index = 0) {
  harness.document.emit('click', { target: harness.buttons[index] });
  await new Promise((resolve) => setImmediate(resolve));
  await new Promise((resolve) => setImmediate(resolve));
}

test('successful signed-in DELETE removes after transition and reveals final empty state', async () => {
  const h = makeHarness();
  await click(h);
  assert.equal(h.cards[0].classList.contains('removing'), true);
  assert.equal(h.cards[0].isConnected, true);
  h.cards[0].emit('transitionend');
  assert.equal(h.cards[0].isConnected, false);
  assert.equal(h.count.textContent, '0');
  assert.equal(h.list.hidden, true);
  assert.equal(h.pageEmpty.hidden, false);
});

test('timeout removes when transitionend never fires', async () => {
  const h = makeHarness();
  await click(h);
  h.timers[0]();
  assert.equal(h.cards[0].isConnected, false);
});

test('HTTP failure restores the icon and leaves page state unchanged', async () => {
  const h = makeHarness({ response: { reject: true } });
  await click(h);
  assert.equal(h.cards[0].isConnected, true);
  assert.equal(h.buttons[0].classList.contains('on'), true);
  assert.equal(h.buttons[0].attributes['aria-pressed'], 'true');
  assert.equal(h.count.textContent, '1');
  assert.equal(h.pageEmpty.hidden, true);
});

test('contradictory final bookmarked true leaves the card', async () => {
  const h = makeHarness({ response: { ok: true, bookmarked: true } });
  await click(h);
  assert.equal(h.cards[0].isConnected, true);
  assert.equal(h.cards[0].classList.contains('removing'), false);
});

test('non-bookmarks route never removes a card', async () => {
  const h = makeHarness({ route: '/' });
  await click(h);
  assert.equal(h.cards[0].isConnected, true);
});

test('one of two removals updates count without showing page empty state', async () => {
  const h = makeHarness({ sources: ['jumpit', 'rallit'] });
  await click(h);
  h.cards[0].emit('transitionend');
  assert.equal(h.count.textContent, '1');
  assert.equal(h.list.hidden, false);
  assert.equal(h.pageEmpty.hidden, true);
});

test('source-filter empty message recomputes from connected cards', async () => {
  const h = makeHarness({ sources: ['jumpit', 'rallit'], filterSource: 'jumpit' });
  await click(h);
  h.cards[0].emit('transitionend');
  assert.equal(h.pageEmpty.hidden, true);
  assert.equal(h.filterEmpty.hidden, false);
  assert.equal(h.filterEmpty.textContent, '저장한 jumpit 공고가 없어요.');
});

test('text-search empty message recomputes from connected cards', async () => {
  const h = makeHarness({
    sources: ['jumpit', 'rallit'],
    titles: ['삭제할 공고', '남은 공고'],
    searchValue: '삭제할',
  });
  await click(h);
  h.cards[0].emit('transitionend');
  assert.equal(h.filterEmpty.hidden, false);
  assert.equal(h.filterEmpty.textContent, '검색 결과가 없어요.');
});

test('demo branch keeps immediate localStorage hiding and adds no transition', async () => {
  const h = makeHarness({ demo: true });
  await click(h);
  assert.equal(h.cards[0].hidden, true);
  assert.equal(h.cards[0].classList.contains('removing'), false);
  assert.equal(JSON.parse(h.storage.get('jobcronDemoBookmarks')).length, 0);
});

test('request completion re-enables the button', async () => {
  const h = makeHarness({ response: { ok: true, bookmarked: true } });
  await click(h);
  assert.equal(h.buttons[0].disabled, false);
});
