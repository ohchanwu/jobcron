// Theme toggle: cycles auto -> light -> dark -> auto, persists to
// localStorage, and swaps the toggle button's icon. Default (no
// data-theme attribute) follows the system via CSS color-scheme.
//
// Icons are inline Lucide (MIT) paths so there is no font dependency.
// stroke="currentColor" inherits the toggle's color, which is driven
// by --ink-soft and shifts with the theme.
(function () {
  var root = document.documentElement;
  var btn = document.getElementById('theme-toggle');
  if (!btn) return;
  var states = ['auto', 'light', 'dark'];
  // aria-label per state — used by screen readers since the SVG itself
  // has no text content. Pairs with the visible tooltip in profile.html
  // and index.html which describes the cycle.
  var ariaLabels = {
    auto:  '현재 테마: 자동 (시스템 설정)',
    light: '현재 테마: 낮',
    dark:  '현재 테마: 밤'
  };
  var svgAttrs =
    'width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor"' +
    ' stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"';
  var icons = {
    // monitor — system-follows-OS metaphor
    auto:
      '<svg ' + svgAttrs + '>' +
        '<rect width="20" height="14" x="2" y="3" rx="2"/>' +
        '<line x1="8" x2="16" y1="21" y2="21"/>' +
        '<line x1="12" x2="12" y1="17" y2="21"/>' +
      '</svg>',
    // sun
    light:
      '<svg ' + svgAttrs + '>' +
        '<circle cx="12" cy="12" r="4"/>' +
        '<path d="M12 2v2"/><path d="M12 20v2"/>' +
        '<path d="m4.93 4.93 1.41 1.41"/><path d="m17.66 17.66 1.41 1.41"/>' +
        '<path d="M2 12h2"/><path d="M20 12h2"/>' +
        '<path d="m6.34 17.66-1.41 1.41"/><path d="m19.07 4.93-1.41 1.41"/>' +
      '</svg>',
    // crescent moon
    dark:
      '<svg ' + svgAttrs + '>' +
        '<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>' +
      '</svg>'
  };

  function current() {
    return root.dataset.theme === 'light' ? 'light' :
           root.dataset.theme === 'dark'  ? 'dark'  : 'auto';
  }
  function apply(t) {
    if (t === 'auto') {
      delete root.dataset.theme;
      try { localStorage.removeItem('theme'); } catch (e) {}
    } else {
      root.dataset.theme = t;
      try { localStorage.setItem('theme', t); } catch (e) {}
    }
    btn.innerHTML = icons[t];
    btn.setAttribute('aria-label', ariaLabels[t]);
  }
  apply(current());

  btn.addEventListener('click', function () {
    var next = states[(states.indexOf(current()) + 1) % 3];
    apply(next);
  });
})();
