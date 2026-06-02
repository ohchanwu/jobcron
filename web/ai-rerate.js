/* 재평가 (re-rate): stream Stage-2 AI deltas for the visible rows of this
   surface, then reload so the refreshed chips render. Mirrors the scrape
   EventSource flow; the server enforces scrape⟷re-rate mutual exclusion (409).
   The button is absent entirely when no AI key is set, so this is a no-op then. */
(function () {
  var btn = document.getElementById('rerate');
  var log = document.getElementById('rerate-log');
  if (!btn || !log) return;

  btn.addEventListener('click', function () {
    var surface = btn.dataset.surface;
    if (!surface) return;
    btn.disabled = true;
    log.textContent = '';
    var es = new EventSource('/api/rerate?surface=' + encodeURIComponent(surface));

    function append(msg) {
      var p = document.createElement('p');
      p.textContent = msg;
      log.appendChild(p);
    }
    function progress(msg) {
      var pr = document.getElementById('rerate-progress');
      if (!pr) {
        pr = document.createElement('p');
        pr.id = 'rerate-progress';
        log.appendChild(pr);
      }
      pr.textContent = msg;
    }

    es.addEventListener('status', function (e) { append(e.data); });
    es.addEventListener('progress', function (e) { progress(e.data); });
    es.addEventListener('done', function () { es.close(); location.reload(); });
    es.addEventListener('failed', function (e) {
      es.close();
      btn.disabled = false;
      append(e.data || '재평가에 실패했어요.');
    });
    es.addEventListener('error', function () {
      es.close();
      btn.disabled = false;
      append('연결이 끊겼어요. 잠시 후 다시 시도해 주세요.');
    });
  });
})();
