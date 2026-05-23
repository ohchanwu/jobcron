/* Bookmark icon: optimistic toggle, server reconciles. One delegated
   click handler covers every .bookmark button on the page, including any
   that show up later (the dashboard re-renders on scrape completion). */
(function () {
  document.addEventListener('click', function (e) {
    var btn = e.target.closest('.bookmark');
    if (!btn || btn.disabled) return;

    var id = btn.dataset.postingId;
    if (!id) return;

    var wasOn = btn.classList.contains('on');
    // Optimistic flip — gives the click instant feedback even on a slow
    // local DB. The server response below is the source of truth.
    btn.classList.toggle('on');
    btn.setAttribute('aria-pressed', String(!wasOn));
    btn.disabled = true;

    fetch('/api/bookmark/' + encodeURIComponent(id), {
      method: wasOn ? 'DELETE' : 'PUT',
    })
      .then(function (r) {
        if (!r.ok) throw new Error('bookmark request failed: ' + r.status);
        return r.json();
      })
      .then(function (state) {
        var on = !!state.bookmarked;
        btn.classList.toggle('on', on);
        btn.setAttribute('aria-pressed', String(on));
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
})();
