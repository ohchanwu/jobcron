/* Source filter — adds a "전체 · 점핏 · 랠릿 · …" pill bar above the
   posting list and toggles posting visibility in-place. Renders only when
   2+ unique sources appear on the page; single-source pages stay clean.

   Filter state is ephemeral (per page load) on purpose — fresh page should
   feel like a fresh briefing, not "the same filter I clicked an hour ago." */
(function () {
  var FILTER_CONTAINER_ID = 'source-filter';
  var ACTIVE_KEY = 'all';

  function init() {
    var container = document.getElementById(FILTER_CONTAINER_ID);
    if (!container) return;

    var postings = document.querySelectorAll('.posting[data-source]');
    if (postings.length === 0) return;

    var labelByID = readSourceLabels(postings);
    var ids = Object.keys(labelByID);
    if (ids.length < 2) return; // single source: no filter needed

    // Stable display order — alphabetical by label so it does not flip
    // between renders.
    ids.sort(function (a, b) { return labelByID[a].localeCompare(labelByID[b]); });

    container.appendChild(buildPill(ACTIVE_KEY, '전체', true));
    ids.forEach(function (id) {
      container.appendChild(buildPill(id, labelByID[id], false));
    });

    container.addEventListener('click', function (e) {
      var pill = e.target.closest('.source-pill');
      if (!pill) return;
      applyFilter(container, pill.dataset.source);
    });
  }

  /* readSourceLabels gathers the unique (id → label) pairs visible on the
     page. We trust the rendered source-badge text — that is the same label
     the server-side template func produced, so we do not need to round-trip
     the mapping into JS. */
  function readSourceLabels(postings) {
    var labels = {};
    postings.forEach(function (p) {
      var id = p.dataset.source;
      if (!id || labels[id]) return;
      var badge = p.querySelector('.source-badge');
      labels[id] = badge ? badge.textContent.trim() : id;
    });
    return labels;
  }

  function buildPill(source, label, active) {
    var btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'source-pill' + (active ? ' on' : '');
    btn.dataset.source = source;
    btn.textContent = label;
    btn.setAttribute('aria-pressed', active ? 'true' : 'false');
    return btn;
  }

  function applyFilter(container, source) {
    container.querySelectorAll('.source-pill').forEach(function (pill) {
      var on = pill.dataset.source === source;
      pill.classList.toggle('on', on);
      pill.setAttribute('aria-pressed', on ? 'true' : 'false');
    });

    document.querySelectorAll('.posting[data-source]').forEach(function (p) {
      var match = source === ACTIVE_KEY || p.dataset.source === source;
      p.classList.toggle('filter-hidden', !match);
    });

    /* Archive page: hide day-group sections that have no visible postings
       after filtering, so the day header does not float over nothing. */
    document.querySelectorAll('.archive-day').forEach(function (day) {
      var visible = day.querySelectorAll('.posting:not(.filter-hidden)').length;
      day.classList.toggle('filter-hidden', visible === 0);
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
