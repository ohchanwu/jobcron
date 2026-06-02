/* AI 분석 evidence popover.
   The chip is a real <button>, so Enter/Space activation is free — this only
   manages aria-expanded and the sibling .ai-evidence [hidden] disclosure.
   Click-to-toggle (not hover: hover fails on touch + keyboard). ESC closes any
   open popover. One delegated listener covers every chip across the briefing. */
(function () {
  function evidenceOf(btn) {
    var wrap = btn.parentElement;
    return wrap ? wrap.querySelector('.ai-evidence') : null;
  }
  function setOpen(btn, open) {
    btn.setAttribute('aria-expanded', open ? 'true' : 'false');
    var ev = evidenceOf(btn);
    if (ev) ev.hidden = !open;
  }
  document.addEventListener('click', function (e) {
    var btn = e.target.closest ? e.target.closest('.chip-ai') : null;
    if (!btn) return;
    setOpen(btn, btn.getAttribute('aria-expanded') !== 'true');
  });
  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Escape' && e.key !== 'Esc') return;
    var open = document.querySelectorAll('.chip-ai[aria-expanded="true"]');
    for (var i = 0; i < open.length; i++) setOpen(open[i], false);
  });
})();
