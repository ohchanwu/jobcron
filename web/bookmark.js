/* Bookmark icon: optimistic toggle, server reconciles. One delegated
   click handler covers every .bookmark button on the page, including any
   that show up later (the dashboard re-renders on scrape completion). */
(function () {
  var KEY = 'jobcronDemoBookmarks';

  function demoMode() {
    return document.body && document.body.dataset.demo === 'true';
  }

  function readSet() {
    try {
      var raw = localStorage.getItem(KEY);
      var arr = raw ? JSON.parse(raw) : [];
      return new Set(Array.isArray(arr) ? arr.map(String) : []);
    } catch (e) {
      return new Set();
    }
  }

  function writeSet(set) {
    try { localStorage.setItem(KEY, JSON.stringify(Array.from(set))); } catch (e) {}
  }

  function paintButton(btn, on) {
    btn.classList.toggle('on', on);
    btn.setAttribute('aria-pressed', String(on));
  }

  function signedInBookmarksPage() {
    return !demoMode() && location.pathname === '/bookmarks';
  }

  function syncSignedInBookmarks() {
    if (!signedInBookmarksPage()) return;
    var cards = Array.from(document.querySelectorAll('.posting'))
      .filter(function (card) { return card.isConnected; });
    var count = document.querySelector('.count strong');
    var list = document.querySelector('ol.postings');
    var empty = document.querySelector('[data-bookmarks-empty]');

    if (count) count.textContent = String(cards.length);
    if (list) list.hidden = cards.length === 0;
    if (empty) empty.hidden = cards.length !== 0;
    document.dispatchEvent(new CustomEvent('posting-list-change'));
  }

  function fadeRemove(el, afterRemove) {
    if (!el) return;
    el.classList.add('removing');
    var done = false;
    function go() {
      if (done) return;
      done = true;
      el.remove();
      if (afterRemove) afterRemove();
    }
    el.addEventListener('transitionend', go, { once: true });
    setTimeout(go, 260);
  }

  function csrfToken() {
    var meta = document.querySelector('meta[name="csrf-token"]');
    return meta ? meta.getAttribute('content') || '' : '';
  }

  function syncDemoBookmarks() {
    if (!demoMode()) return;
    var saved = readSet();
    document.querySelectorAll('.bookmark[data-posting-id]').forEach(function (btn) {
      paintButton(btn, saved.has(String(btn.dataset.postingId)));
    });
    if (location.pathname === '/bookmarks') {
      document.querySelectorAll('.posting').forEach(function (card) {
        var btn = card.querySelector('.bookmark[data-posting-id]');
        card.hidden = !btn || !saved.has(String(btn.dataset.postingId));
      });
      updateDemoCount('저장된 공고');
    }
    document.dispatchEvent(new CustomEvent('demo-state-change'));
  }

  function updateDemoCount(label) {
    var visible = document.querySelectorAll('.posting:not([hidden])').length;
    var count = document.querySelector('.count strong');
    if (count) count.textContent = String(visible);
    var empty = document.querySelector('.empty');
    if (empty) empty.hidden = visible !== 0;
  }

  document.addEventListener('click', function (e) {
    var btn = e.target.closest('.bookmark');
    if (!btn || btn.disabled) return;

    var id = btn.dataset.postingId;
    if (!id) return;

    var wasOn = btn.classList.contains('on');
    if (demoMode()) {
      var saved = readSet();
      if (wasOn) saved.delete(String(id)); else saved.add(String(id));
      writeSet(saved);
      syncDemoBookmarks();
      return;
    }
    // Optimistic flip — gives the click instant feedback even on a slow
    // local DB. The server response below is the source of truth.
    btn.classList.toggle('on');
    btn.setAttribute('aria-pressed', String(!wasOn));
    btn.disabled = true;

    fetch('/api/bookmark/' + encodeURIComponent(id), {
      method: wasOn ? 'DELETE' : 'PUT',
      headers: { 'X-CSRF-Token': csrfToken() },
    })
      .then(function (r) {
        if (!r.ok) throw new Error('bookmark request failed: ' + r.status);
        return r.json();
      })
      .then(function (state) {
        var on = !!state.bookmarked;
        btn.classList.toggle('on', on);
        btn.setAttribute('aria-pressed', String(on));
        if (!on && signedInBookmarksPage()) {
          fadeRemove(btn.closest('.posting'), syncSignedInBookmarks);
        }
      })
      .catch(function () {
        // Roll back the optimistic flip so the icon matches reality.
        btn.classList.toggle('on', wasOn);
        btn.setAttribute('aria-pressed', String(wasOn));
      })
      .finally(function () {
        btn.disabled = false;
      });
  });

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', syncDemoBookmarks);
  } else {
    syncDemoBookmarks();
  }
})();
