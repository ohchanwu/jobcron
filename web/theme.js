// Theme toggle: cycles auto -> light -> dark -> auto, persists to
// localStorage, and updates the toggle button's label. Default (no
// data-theme attribute) follows the system via CSS color-scheme.
(function () {
  var root = document.documentElement;
  var btn = document.getElementById('theme-toggle');
  if (!btn) return;
  var states = ['auto', 'light', 'dark'];
  var labels = { auto: 'AUTO', light: 'LIGHT', dark: 'DARK' };

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
    btn.textContent = labels[t];
  }
  apply(current());

  btn.addEventListener('click', function () {
    var next = states[(states.indexOf(current()) + 1) % 3];
    apply(next);
  });
})();
