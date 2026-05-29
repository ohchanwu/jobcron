/* "관심 없음" mute toggle: optimistic, server reconciles. One delegated
   click handler covers every .not-interested button on the page, including
   any added later when the dashboard re-renders on scrape completion.

   Context-aware removal, so the click feels finished:
   - On the briefing (/) and 관심 공고 (/archive), muting a posting removes
     its card — those surfaces hide muted postings entirely. On /bookmarks a
     muted posting stays (it is bookmarked), so the card is kept and only the
     icon state flips.
   - In the profile page's 관심 없음 list, un-muting removes the row. */
(function () {
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
    btn.disabled = true;

    fetch('/api/not-interested/' + encodeURIComponent(id), {
      method: wasOn ? 'DELETE' : 'PUT',
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
        var item = btn.closest('.muted-item');
        if (muted && card && location.pathname !== '/bookmarks') {
          fadeRemove(card); // muted: leaves the briefing / 관심 공고 view
        } else if (!muted && item) {
          fadeRemove(item); // un-muted from the profile list
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
})();
