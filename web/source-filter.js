/* Source filter — works with server-rendered pills.

   Pills are rendered by the template for every registered source plus an
   always-present "전체" pill. This script:
     1. Marks each non-전체 pill `.empty` when no visible posting matches
        its source (so the user can SEE the source exists but knows it
        has nothing right now).
     2. Handles click: filter visible postings in-place, and when the
        filter selection yields zero matches, surface a page-specific
        empty message ("오늘 X 공고가 없어요" / "X 공고가 없어요" /
        "저장한 X 공고가 없어요"). The message format string comes from
        the container's data-empty-template attribute, with {label}
        substituted at click time.

   Filter state is intentionally ephemeral (no localStorage) — opening
   the page should feel like a fresh briefing, not "the filter I left
   on yesterday." */
(function () {
  var ALL_KEY = '_all';

  function init() {
    var container = document.getElementById('source-filter');
    if (!container) return;
    var emptyMsg = document.getElementById('source-filter-empty');
    var emptyTemplate = container.dataset.emptyTemplate || '';

    var postings = document.querySelectorAll('.posting[data-source]');
    markEmptyPills(container, postings);

    container.addEventListener('click', function (e) {
      var pill = e.target.closest('.source-pill');
      if (!pill) return;
      applyFilter(pill.dataset.source, container, emptyMsg, emptyTemplate);
    });
  }

  /* markEmptyPills counts how many postings each source has on this page
     and marks the source's pill `.empty` when the count is zero. Empty
     pills stay clickable (the user might want to confirm it's empty) but
     are visually muted via CSS. */
  function markEmptyPills(container, postings) {
    var counts = {};
    postings.forEach(function (p) {
      /* data-source carries a CSV of every source the posting represents
         — the canonical's own source plus any duplicates collapsed onto
         it. Each source counts as one occurrence for pill-emptiness. */
      p.dataset.source.split(',').forEach(function (id) {
        if (id) counts[id] = (counts[id] || 0) + 1;
      });
    });
    container.querySelectorAll('.source-pill').forEach(function (pill) {
      var src = pill.dataset.source;
      if (src === ALL_KEY) return; // "전체" is never empty in this sense
      pill.classList.toggle('pill-empty', !counts[src]);
    });
  }

  function applyFilter(source, container, emptyMsg, emptyTemplate) {
    container.querySelectorAll('.source-pill').forEach(function (pill) {
      var on = pill.dataset.source === source;
      pill.classList.toggle('on', on);
      pill.setAttribute('aria-pressed', on ? 'true' : 'false');
    });

    var anyVisible = false;
    var allPostings = document.querySelectorAll('.posting[data-source]');
    allPostings.forEach(function (p) {
      /* Posting matches the filter when the selected source is in its
         CSV — covers cross-portal duplicates collapsed onto a canonical
         from a different source. */
      var match = source === ALL_KEY ||
        (',' + p.dataset.source + ',').indexOf(',' + source + ',') !== -1;
      p.classList.toggle('filter-hidden', !match);
      if (match) anyVisible = true;
    });

    /* Archive page: hide day-group sections whose postings all got
       filtered away so the date header does not float over nothing. */
    document.querySelectorAll('.archive-day').forEach(function (day) {
      var visible = day.querySelectorAll('.posting:not(.filter-hidden)').length;
      day.classList.toggle('filter-hidden', visible === 0);
    });

    /* Show / hide the filter-induced empty message. The page's own
       no-postings-at-all empty state (rendered when the data is empty
       to begin with) is separate; we only show ours when the filter
       narrowed a non-empty page down to zero. */
    /* Only fire the filter-empty message when the page has SOME postings
       (the filter just hides them all). When the page is empty to begin
       with, its own empty-state block ("아직 ... 공고가 없어요") already
       handles it — duplicating would be noise. */
    if (!emptyMsg) return;
    var pageHasPostings = allPostings.length > 0;
    if (source === ALL_KEY || anyVisible || !pageHasPostings) {
      emptyMsg.hidden = true;
      emptyMsg.textContent = '';
    } else {
      var label = labelFor(container, source);
      emptyMsg.textContent = emptyTemplate.replace('{label}', label);
      emptyMsg.hidden = false;
    }
  }

  function labelFor(container, source) {
    var pill = container.querySelector('.source-pill[data-source="' + source + '"]');
    return pill ? pill.textContent.trim() : source;
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
