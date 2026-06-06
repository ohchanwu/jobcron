/* Provider-aware model dropdown. When the user changes the AI provider select,
   repopulate the model select with that provider's models (plus a "기본값"
   option that maps to the empty value → the server's per-provider default).
   Toggling the provider to "없음" (off) empties the model list. The full
   provider→models map is injected by the template as window.aiModelOptions.
   (Anthropic is currently the only provider; the swap stays provider-aware so a
   second provider would just slot back in.) */
(function () {
  var options = window.aiModelOptions || {};
  var provider = document.getElementById('ai-provider-select');
  var model = document.getElementById('ai-model-select');
  if (!provider || !model) return;

  provider.addEventListener('change', function () {
    var models = options[provider.value] || [];
    model.innerHTML = '';

    var def = document.createElement('option');
    def.value = '';
    def.textContent = '기본값';
    model.appendChild(def);

    models.forEach(function (id) {
      var opt = document.createElement('option');
      opt.value = id;
      opt.textContent = id;
      model.appendChild(opt);
    });
    // After a provider switch the previous model id is almost never valid for
    // the new provider, so default to "기본값" (empty) — which the server resolves
    // to the new provider's default model. The user can pick a specific one.
    model.value = '';
  });
})();
