/* Posting filters — source pills + text search, composed (AND).

   Two filters narrow the visible postings together:
     1. Source pills (전체 / 점핏 / 랠릿 / …), rendered by the template for every
        registered source. Clicking one shows only that source.
     2. A text search box (#posting-search, present on /, /archive, /bookmarks):
        case-insensitive substring over each posting's title + company. This is
        a "find it again" affordance, NOT the scoring matcher — plain substring
        (including Korean) is intentional here.

   A posting is visible only when it matches BOTH the active source AND the
   query, so the two compose. Everything downstream — empty day-group sections,
   the 관심 밖 collapsible (open state + count), and the no-results message — is
   recomputed from the combined visible set in one place, applyFilters(). A
   single chokepoint is deliberate: two independent scripts each toggling
   .filter-hidden would fight.

   Filter state is intentionally ephemeral (no localStorage) — opening a page
   should feel like a fresh briefing, not yesterday's leftover filter. */
(function () {
  var ALL_KEY = '_all';

  function init() {
    var container = document.getElementById('source-filter');
    if (!container) return;
    var emptyMsg = document.getElementById('source-filter-empty');
    var emptyTemplate = container.dataset.emptyTemplate || '';
    var searchInput = document.getElementById('posting-search');

    /* Cache each posting's lowercased search text (title + company) once, so a
       keystroke does not re-read the DOM for every card. NFC-normalized so a
       query and a title that differ only by Unicode composition still match. */
    var cards = [];
    document.querySelectorAll('.posting[data-source]').forEach(function (el) {
      var title = el.querySelector('.posting-title');
      var company = el.querySelector('.posting-meta span');
      var text = ((title ? title.textContent : '') + ' ' +
                  (company ? company.textContent : '')).normalize('NFC').toLowerCase();
      cards.push({ el: el, text: text });
    });

    /* The 관심 밖 collapsible (only on / and /archive) and its count span. We
       snapshot the box's open state when a search becomes active and restore it
       when the search clears, so search can drive the box open/closed during a
       query without losing whatever the user had set before they typed. */
    var excludedBox = document.querySelector('.excluded-box');
    var countEl = excludedBox ? excludedBox.querySelector('.excluded-count') : null;
    var originalCount = countEl ? countEl.textContent : null;
    var preSearchOpen = null;

    markEmptyPills(container, cards);

    container.addEventListener('click', function (e) {
      var pill = e.target.closest('.source-pill');
      if (!pill) return;
      setActivePill(container, pill.dataset.source);
      applyFilters();
    });
    if (searchInput) {
      searchInput.addEventListener('input', applyFilters);
    }

    function activeSource() {
      var on = container.querySelector('.source-pill.on');
      return on ? on.dataset.source : ALL_KEY;
    }

    function applyFilters() {
      var source = activeSource();
      var q = searchInput ? searchInput.value.trim().normalize('NFC').toLowerCase() : '';

      var anyVisible = false;
      cards.forEach(function (c) {
        /* Source match: the selected source is in the posting's CSV (covers
           cross-portal duplicates collapsed onto a canonical from elsewhere). */
        var srcMatch = source === ALL_KEY ||
          (',' + c.el.dataset.source + ',').indexOf(',' + source + ',') !== -1;
        var qMatch = q === '' || c.text.indexOf(q) !== -1;
        var visible = srcMatch && qMatch;
        c.el.classList.toggle('filter-hidden', !visible);
        if (visible) anyVisible = true;
      });

      /* Archive: hide day-group sections whose postings all got filtered away,
         so a date header does not float over nothing. */
      document.querySelectorAll('.archive-day').forEach(function (day) {
        var visible = day.querySelectorAll('.posting:not(.filter-hidden)').length;
        day.classList.toggle('filter-hidden', visible === 0);
      });

      updateExcludedBox(q);
      updateEmptyMessage(source, q, anyVisible);
    }

    /* The 관심 밖 collapsible needs care during search: a matching low-score
       card lives in the DOM inside a closed <details>, so it must be opened to
       be seen, and the server-rendered count must reflect the filtered subset. */
    function updateExcludedBox(q) {
      if (!excludedBox) return;
      if (q !== '') {
        // Snapshot the user's open state once, when the search begins.
        if (preSearchOpen === null) preSearchOpen = excludedBox.open;
        var visibleInside = excludedBox.querySelectorAll('.posting:not(.filter-hidden)').length;
        // Search-driven: open the box iff a match lives inside it — so a match
        // is never stranded behind a collapsed summary, and the box never stays
        // expanded over zero matching content (even if the user opened it).
        excludedBox.open = visibleInside > 0;
        if (countEl) countEl.textContent = visibleInside;
      } else {
        // Search cleared: restore exactly what the user had before they typed.
        if (preSearchOpen !== null) {
          excludedBox.open = preSearchOpen;
          preSearchOpen = null;
        }
        if (countEl && originalCount !== null) countEl.textContent = originalCount;
      }
    }

    /* The filter-induced empty message is separate from the page's own
       no-postings-at-all empty state (rendered server-side). We only show ours
       when the active filters narrowed a non-empty page down to zero. */
    function updateEmptyMessage(source, q, anyVisible) {
      if (!emptyMsg) return;
      var pageHasPostings = cards.length > 0;
      if (anyVisible || !pageHasPostings) {
        emptyMsg.hidden = true;
        emptyMsg.textContent = '';
        return;
      }
      if (q !== '') {
        emptyMsg.textContent = '검색 결과가 없어요.';
        emptyMsg.hidden = false;
      } else if (source !== ALL_KEY) {
        emptyMsg.textContent = emptyTemplate.replace('{label}', labelFor(container, source));
        emptyMsg.hidden = false;
      } else {
        emptyMsg.hidden = true;
        emptyMsg.textContent = '';
      }
    }
  }

  /* setActivePill moves the `on` state to the clicked pill. */
  function setActivePill(container, source) {
    container.querySelectorAll('.source-pill').forEach(function (pill) {
      var on = pill.dataset.source === source;
      pill.classList.toggle('on', on);
      pill.setAttribute('aria-pressed', on ? 'true' : 'false');
    });
  }

  /* markEmptyPills marks a source's pill `.pill-empty` when this page has no
     posting from that source. Search-independent: pill emptiness is about which
     sources exist on the page, not what the current query matches. */
  function markEmptyPills(container, cards) {
    var counts = {};
    cards.forEach(function (c) {
      c.el.dataset.source.split(',').forEach(function (id) {
        if (id) counts[id] = (counts[id] || 0) + 1;
      });
    });
    container.querySelectorAll('.source-pill').forEach(function (pill) {
      var src = pill.dataset.source;
      if (src === ALL_KEY) return; // "전체" is never empty in this sense
      pill.classList.toggle('pill-empty', !counts[src]);
    });
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
