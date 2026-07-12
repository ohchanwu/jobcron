(function () {
  var btn = document.getElementById('rerate');
  var log = document.getElementById('rerate-log');
  var activity = document.getElementById('rerate-activity');
  if (!btn || !log || !activity) return;

  var surface = btn.dataset.surface;
  if (!surface) return;
  var eventSource = null;
  var pollTimer = null;
  var activeRunID = 0;
  var noticeKey = 'jobcron:rerate-notice:' + surface;
  var handledKey = 'jobcron:rerate-handled:' + surface;
  var activeCopy = 'AI로 다시 분석하는 중이에요 — 여러 공고를 한 번에 살펴보고 있어요. ☕';
  var completedAwayCopy = 'AI 평가가 완료됐어요. 새로운 평가 결과를 반영했습니다.';

  function messageElement(id) {
    var node = document.getElementById(id);
    if (!node) {
      node = document.createElement('p');
      node.id = id;
      log.appendChild(node);
    }
    return node;
  }

  function setMessage(node, msg) {
    node.textContent = '';
    var settingsText = '프로필 설정';
    var index = msg.indexOf(settingsText);
    if (index === -1) {
      node.textContent = msg;
      return;
    }
    node.appendChild(document.createTextNode(msg.slice(0, index)));
    var link = document.createElement('a');
    link.href = '/profile';
    link.className = 'budget-settings-link';
    link.textContent = settingsText;
    node.appendChild(link);
    node.appendChild(document.createTextNode(msg.slice(index + settingsText.length)));
  }

  function showStatus(msg) {
    if (msg) setMessage(messageElement('rerate-status'), msg);
  }

  function showProgress(msg) {
    if (msg) messageElement('rerate-progress').textContent = msg;
  }

  function setRunning(running) {
    btn.disabled = running;
    activity.hidden = !running;
  }

  function stopTransport() {
    if (eventSource) {
      eventSource.close();
      eventSource = null;
    }
    if (pollTimer) {
      clearTimeout(pollTimer);
      pollTimer = null;
    }
  }

  function rememberAndReload(message, runID) {
    if (runID) sessionStorage.setItem(handledKey, String(runID));
    sessionStorage.setItem(noticeKey, message);
    location.reload();
  }

  function showStoredNotice() {
    var message = sessionStorage.getItem(noticeKey);
    if (!message) return;
    sessionStorage.removeItem(noticeKey);
    showStatus(message);
  }

  function pollStatus() {
    fetch('/api/rerate/status?surface=' + encodeURIComponent(surface), {
      headers: { 'Accept': 'application/json' },
      cache: 'no-store'
    }).then(function (response) {
      if (!response.ok) throw new Error('status ' + response.status);
      return response.json();
    }).then(function (status) {
      var handled = status.run_id && sessionStorage.getItem(handledKey) === String(status.run_id);
      if (status.state === 'running') {
        setRunning(true);
        showStatus(status.status || activeCopy);
        showProgress(status.progress || '공고 분석을 준비하는 중...');
        pollTimer = setTimeout(pollStatus, 750);
        return;
      }
      setRunning(false);
      if (status.state === 'done' && !handled) {
        rememberAndReload(completedAwayCopy, status.run_id);
        return;
      }
      if (status.state === 'failed' && !handled) {
        sessionStorage.setItem(handledKey, String(status.run_id));
        showStatus(status.message || 'AI 평가에 실패했어요.');
      }
    }).catch(function () {
      setRunning(false);
      showStatus('진행 상태를 다시 불러오지 못했어요. 잠시 후 다시 시도해 주세요.');
    });
  }

  function isHistoryReturn(event) {
    if (event && event.persisted) return true;
    var entries = performance.getEntriesByType ? performance.getEntriesByType('navigation') : [];
    return entries.length > 0 && entries[0].type === 'back_forward';
  }

  btn.addEventListener('click', function () {
    stopTransport();
    log.textContent = '';
    setRunning(true);
    showStatus(activeCopy);
    eventSource = new EventSource('/api/rerate?surface=' + encodeURIComponent(surface));
    eventSource.addEventListener('run', function (event) { activeRunID = Number(event.data) || 0; });
    eventSource.addEventListener('status', function (event) { showStatus(event.data); });
    eventSource.addEventListener('progress', function (event) { showProgress(event.data); });
    eventSource.addEventListener('done', function (event) {
      stopTransport();
      setRunning(false);
      rememberAndReload(event.data, activeRunID);
    });
    eventSource.addEventListener('failed', function (event) {
      stopTransport();
      setRunning(false);
      showStatus(event.data || 'AI 평가에 실패했어요.');
    });
    eventSource.addEventListener('error', function () {
      if (document.visibilityState === 'hidden') return;
      stopTransport();
      setRunning(false);
      showStatus('연결이 끊겼어요. 잠시 후 다시 시도해 주세요.');
    });
  });

  window.addEventListener('pagehide', stopTransport);
  window.addEventListener('pageshow', function (event) {
    showStoredNotice();
    if (isHistoryReturn(event)) pollStatus();
  });
  showStoredNotice();
})();
