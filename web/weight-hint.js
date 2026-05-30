/* Live-preview the derived near-miss / ambiguous awards as the user edits a
   weight, so the hint matches what scoring will do with the new value.
   Rounding mirrors internal/scoring/rules.go: round-half-up via +den/2,
   i.e. floor((w*num + floor(den/2)) / den). The server renders the initial
   value, so the hint is correct even before this script runs. */
(function () {
  function bind(name, num, den, outId) {
    var input = document.querySelector('input[name="' + name + '"]');
    var out = document.getElementById(outId);
    if (!input || !out) return;
    input.addEventListener('input', function () {
      var w = parseInt(input.value, 10);
      if (isNaN(w) || w < 0) w = 0;
      out.textContent = Math.floor((w * num + Math.floor(den / 2)) / den);
    });
  }
  bind('career_weight', 2, 5, 'derive-career'); // round(w × 2/5)
  bind('salary_weight', 1, 2, 'derive-salary'); // round(w ÷ 2)
})();
