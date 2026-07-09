/* "관심 없음" mute toggle: optimistic, server reconciles. One delegated
   click handler covers every .not-interested button on the page, including
   any added later when the dashboard re-renders on scrape completion.

   Context-aware removal, so the click feels finished:
   - On the briefing (/) and 전체 공고 (/archive), muting a posting removes
     its card — those surfaces hide muted postings entirely. On /bookmarks a
     muted posting stays (it is bookmarked), so the card is kept and only the
     icon state flips.
   - On the 숨긴 공고 page (/hidden), every card is muted; un-muting one means
     it no longer belongs there, so fade the .posting card out. */
(function () {
  var KEY = 'jobScraperDemoHidden';

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

  function csrfToken() {
    var meta = document.querySelector('meta[name="csrf-token"]');
    return meta ? meta.getAttribute('content') || '' : '';
  }

  function syncDemoHidden() {
    if (!demoMode()) return;
    var hidden = readSet();
    document.querySelectorAll('.not-interested[data-posting-id]').forEach(function (btn) {
      paintButton(btn, hidden.has(String(btn.dataset.postingId)));
    });
    document.querySelectorAll('.posting').forEach(function (card) {
      var btn = card.querySelector('.not-interested[data-posting-id]');
      if (!btn) return;
      var isHidden = hidden.has(String(btn.dataset.postingId));
      if (location.pathname === '/hidden') {
        card.hidden = !isHidden;
      } else if (location.pathname !== '/bookmarks') {
        card.hidden = isHidden;
      }
    });
    if (location.pathname === '/hidden') updateDemoCount();
  }

  function updateDemoCount() {
    var visible = document.querySelectorAll('.posting:not([hidden])').length;
    var count = document.querySelector('.count strong');
    if (count) count.textContent = String(visible);
    var empty = document.querySelector('.empty');
    if (empty) empty.hidden = visible !== 0;
  }

  function fadeRemove(el) {
    if (!el) return;
    el.classList.add('removing');
    // Matches the .removing opacity transition in styles.css; fall back to a
    // hard remove if the transition never fires (e.g. reduced-motion).
    var done = false;
    function go() {
      if (done) return;
      done = true;
      el.remove();
    }
    el.addEventListener('transitionend', go, { once: true });
    setTimeout(go, 260);
  }

  document.addEventListener('click', function (e) {
    var btn = e.target.closest('.not-interested');
    if (!btn || btn.disabled) return;

    var id = btn.dataset.postingId;
    if (!id) return;

    var wasOn = btn.classList.contains('on');
    if (demoMode()) {
      var hidden = readSet();
      if (wasOn) hidden.delete(String(id)); else hidden.add(String(id));
      writeSet(hidden);
      syncDemoHidden();
      return;
    }
    btn.disabled = true;

    fetch('/api/not-interested/' + encodeURIComponent(id), {
      method: wasOn ? 'DELETE' : 'PUT',
      headers: { 'X-CSRF-Token': csrfToken() },
    })
      .then(function (r) {
        if (!r.ok) throw new Error('not-interested request failed: ' + r.status);
        return r.json();
      })
      .then(function (state) {
        var muted = !!state.not_interested;
        btn.classList.toggle('on', muted);
        btn.setAttribute('aria-pressed', String(muted));

        var card = btn.closest('.posting');
        if (muted && card && location.pathname !== '/bookmarks') {
          fadeRemove(card); // muted: leaves the briefing / 전체 공고 view
        } else if (!muted && location.pathname === '/hidden' && card) {
          fadeRemove(card); // un-hidden: leaves the 숨긴 공고 page
        }
      })
      .catch(function () {
        // Reconcile the icon back to its pre-click state on failure.
        btn.classList.toggle('on', wasOn);
        btn.setAttribute('aria-pressed', String(wasOn));
      })
      .finally(function () {
        btn.disabled = false;
      });
  });

  document.addEventListener('demo-state-change', syncDemoHidden);
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', syncDemoHidden);
  } else {
    syncDemoHidden();
  }
})();
