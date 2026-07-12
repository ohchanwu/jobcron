(function () {
  var btn = document.getElementById('rerate');
  var log = document.getElementById('rerate-log');
  var activity = document.getElementById('rerate-activity');
  if (!btn || !log || !activity) return;

  var surface = btn.dataset.surface;
  if (!surface) return;
  var entryStateKey = 'jobcronRerateEntry';
  var eventSource = null;
  var pollTimer = null;
  var statusController = null;
  var lifecycleGeneration = 0;
  var activeRunToken = '';
  var noticeKey = 'jobcron:rerate-notice:' + surface;
  var handledKey = 'jobcron:rerate-handled:' + surface;
  var activeCopy = 'AI로 다시 분석하는 중이에요 — 여러 공고를 한 번에 살펴보고 있어요. ☕';
  var completedAwayCopy = 'AI 평가가 완료됐어요. 새로운 평가 결과를 반영했습니다.';

  function newEntryToken() {
    if (window.crypto && typeof window.crypto.randomUUID === 'function') {
      return window.crypto.randomUUID();
    }
    return Date.now().toString(36) + '-' + Math.random().toString(36).slice(2);
  }

  function ensureEntryToken() {
    var current = history.state;
    var state = current && typeof current === 'object' ? current : {};
    if (state[entryStateKey]) return String(state[entryStateKey]);
    var token = newEntryToken();
    var nextState = {};
    Object.keys(state).forEach(function (key) { nextState[key] = state[key]; });
    nextState[entryStateKey] = token;
    history.replaceState(nextState, document.title);
    return token;
  }

  var entryToken = ensureEntryToken();

  function clearLegacyOwnerKeys() {
    var prefix = 'jobcron:rerate-owner:' + surface + ':';
    for (var i = sessionStorage.length - 1; i >= 0; i--) {
      var key = sessionStorage.key(i);
      if (key && key.indexOf(prefix) === 0) sessionStorage.removeItem(key);
    }
  }

  function ownsStatus(status) {
    return Boolean(status && status.run_token && status.owner_entry === entryToken);
  }

  function isHandled(runToken) {
    return Boolean(runToken) && sessionStorage.getItem(handledKey) === String(runToken);
  }

  function markHandled(runToken) {
    if (runToken) sessionStorage.setItem(handledKey, String(runToken));
  }

  clearLegacyOwnerKeys();

  function messageElement(id) {
    var node = document.getElementById(id);
    if (!node) {
      node = document.createElement('p');
      node.id = id;
      log.appendChild(node);
    }
    return node;
  }

  function removeMessage(id) {
    var node = document.getElementById(id);
    if (node && node.parentNode) node.parentNode.removeChild(node);
  }

  function clearStatus() {
    removeMessage('rerate-status');
  }

  function clearProgress() {
    removeMessage('rerate-progress');
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

  function isCurrent(generation) {
    return generation === lifecycleGeneration;
  }

  function stopTransport() {
    lifecycleGeneration++;
    if (eventSource) {
      eventSource.close();
      eventSource = null;
    }
    if (pollTimer) {
      clearTimeout(pollTimer);
      pollTimer = null;
    }
    if (statusController) {
      statusController.abort();
      statusController = null;
    }
    return lifecycleGeneration;
  }

  function rememberAndReload(message, runToken, ownerEntry) {
    if (!runToken || ownerEntry !== entryToken) return;
    markHandled(runToken);
    sessionStorage.setItem(noticeKey, JSON.stringify({
      entry_token: entryToken,
      run_token: String(runToken),
      message: message
    }));
    location.reload();
  }

  function showStoredNotice() {
    var raw = sessionStorage.getItem(noticeKey);
    if (!raw) return;
    var notice;
    try {
      notice = JSON.parse(raw);
    } catch (error) {
      sessionStorage.removeItem(noticeKey);
      return;
    }
    if (!notice || !notice.run_token) {
      sessionStorage.removeItem(noticeKey);
      return;
    }
    if (notice.entry_token !== entryToken) return;
    if (!isHandled(notice.run_token)) {
      sessionStorage.removeItem(noticeKey);
      return;
    }
    sessionStorage.removeItem(noticeKey);
    showStatus(notice.message);
  }

  function pollStatus(generation) {
    if (!isCurrent(generation)) return;
    var controller = new AbortController();
    statusController = controller;
    fetch('/api/rerate/status?surface=' + encodeURIComponent(surface), {
      headers: { 'Accept': 'application/json' },
      cache: 'no-store',
      signal: controller.signal
    }).then(function (response) {
      if (!isCurrent(generation)) return null;
      if (!response.ok) throw new Error('status ' + response.status);
      return response.json();
    }).then(function (status) {
      if (!isCurrent(generation)) return;
      if (statusController === controller) statusController = null;
      if (!status) return;

      var handled = isHandled(status.run_token);
      if (status.state === 'running') {
        if (!ownsStatus(status)) {
          setRunning(false);
          clearStatus();
          clearProgress();
          return;
        }
        setRunning(true);
        showStatus(status.status || activeCopy);
        showProgress(status.progress || '공고 분석을 준비하는 중...');
        pollTimer = setTimeout(function () {
          if (!isCurrent(generation)) return;
          pollTimer = null;
          pollStatus(generation);
        }, 750);
        return;
      }

      setRunning(false);
      clearProgress();
      if (status.state === 'idle') {
        clearStatus();
        return;
      }
      if (!ownsStatus(status)) {
        clearStatus();
        return;
      }
      if (status.state === 'done') {
        if (!handled) {
          var message = status.outcome === 'changed' ? completedAwayCopy : status.message;
          rememberAndReload(message || completedAwayCopy, status.run_token, status.owner_entry);
          return;
        }
        if (handled) clearStatus();
        return;
      }
      if (status.state === 'failed') {
        if (handled) {
          clearStatus();
          return;
        }
        markHandled(status.run_token);
        showStatus(status.message || 'AI 평가에 실패했어요.');
        return;
      }
      clearStatus();
    }).catch(function (error) {
      if (!isCurrent(generation)) return;
      if (statusController === controller) statusController = null;
      if (error && error.name === 'AbortError') return;
      setRunning(false);
      clearProgress();
      showStatus('진행 상태를 다시 불러오지 못했어요. 잠시 후 다시 시도해 주세요.');
    });
  }

  function isHistoryReturn(event) {
    if (event && event.persisted) return true;
    var entries = performance.getEntriesByType ? performance.getEntriesByType('navigation') : [];
    return entries.length > 0 && entries[0].type === 'back_forward';
  }

  btn.addEventListener('click', function () {
    var generation = stopTransport();
    activeRunToken = '';
    log.textContent = '';
    setRunning(true);
    showStatus(activeCopy);
    var source = new EventSource('/api/rerate?surface=' + encodeURIComponent(surface) +
      '&entry=' + encodeURIComponent(entryToken));
    eventSource = source;
    source.addEventListener('run-token', function (event) {
      if (!isCurrent(generation)) return;
      activeRunToken = event.data || '';
    });
    source.addEventListener('status', function (event) {
      if (!isCurrent(generation)) return;
      showStatus(event.data);
    });
    source.addEventListener('progress', function (event) {
      if (!isCurrent(generation)) return;
      showProgress(event.data);
    });
    source.addEventListener('done', function (event) {
      if (!isCurrent(generation)) return;
      var runToken = activeRunToken;
      stopTransport();
      setRunning(false);
      clearProgress();
      rememberAndReload(event.data, runToken, entryToken);
    });
    source.addEventListener('failed', function (event) {
      if (!isCurrent(generation)) return;
      var runToken = activeRunToken;
      stopTransport();
      setRunning(false);
      clearProgress();
      markHandled(runToken);
      showStatus(event.data || 'AI 평가에 실패했어요.');
    });
    source.addEventListener('error', function () {
      if (!isCurrent(generation)) return;
      if (document.visibilityState === 'hidden') return;
      stopTransport();
      setRunning(false);
      clearProgress();
      showStatus('연결이 끊겼어요. 잠시 후 다시 시도해 주세요.');
    });
  });

  window.addEventListener('pagehide', stopTransport);
  window.addEventListener('pageshow', function (event) {
    showStoredNotice();
    if (isHistoryReturn(event)) pollStatus(stopTransport());
  });
  showStoredNotice();
})();
