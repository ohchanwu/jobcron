(function () {
  var btn = document.getElementById('rerate');
  var log = document.getElementById('rerate-log');
  var activity = document.getElementById('rerate-activity');
  if (!btn || !log || !activity) return;

  var surface = btn.dataset.surface;
  if (!surface) return;
  var entryStateKey = 'jobcronRerateEntry';
  var owner = null;
  var noticeKey = 'jobcron:rerate-notice:' + surface;
  var handledKey = 'jobcron:rerate-handled:' + surface;
  var freshNotice = false;
  var activeCopy = 'AI로 공고를 다시 분석하고 있어요. 약 5–10분 정도 걸릴 수 있어요. 잠시 커피를 마시거나 다른 일을 하고 오셔도 좋아요. ☕ 공고 수와 AI 응답 속도에 따라 더 오래 걸릴 수 있어요.';
  var estimateCopy = activeCopy;
  var completedAwayCopy = 'AI 평가가 완료됐어요. 새로운 평가 결과를 반영했습니다.';
  // Bounded status-only recovery knobs: while an OWNED stream stays open but
  // silent longer than streamSilenceMs, one status probe checks whether the
  // detached run advanced; failed recovery fetches retry up to
  // maxStatusRetries times every statusRetryMs, then stop with an explicit
  // unresolved message. Never a second EventSource (no second provider run).
  var streamSilenceMs = 15000;
  var statusRetryMs = 3000;
  var maxStatusRetries = 20;
  var unresolvedCopy = '진행 상태를 확인하지 못했어요. 잠시 후 페이지를 새로고침해 주세요.';
  // Every status request (poll or probe) carries a deadline covering BOTH the
  // fetch and the body read: a hung response is a transport fault like any
  // other, so its expiry must flow into the same bounded retry/exhaustion
  // path as a rejection — never an open-ended hang with the page disabled.
  var statusDeadlineMs = 10000;


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

  // One owner holds the identity and every asynchronous producer for a run.
  // Every callback enters through admit; settlement invalidates them together.
  function admit(run, record, identity, source) {
    if (owner !== run || run.settled || (record && run.request !== record) ||
        (source && run.source !== source)) return false;
    if (identity) {
      if (!ownsStatus(identity) || (run.token && run.token !== identity.run_token)) {
        var mismatch = ownsStatus(identity) && Boolean(run.token);
        settle(run);
        if (mismatch) {
          showStatus('이전 요청이 종료됐어요. 최근 상태는 새로고침하면 확인할 수 있어요.');
        } else {
          clearProgress();
          if (!freshNotice) clearStatus();
        }
        return false;
      }
      if (!run.token) run.token = identity.run_token;
    }
    return true;
  }

  function clearTimer(run) {
    if (run.timer !== null) clearTimeout(run.timer);
    run.timer = null;
  }

  function cancelRead(run) {
    var record = run.request;
    run.request = null; // invalidate BEFORE abort's promise callbacks
    if (!record) return;
    clearTimeout(record.deadline);
    record.controller.abort();
  }

  function settle(run) {
    run.settled = true;
    clearTimer(run);
    cancelRead(run);
    if (run.source) run.source.close();
    run.source = null;
    setRunning(false);
  }

  function beginOwner() {
    if (owner) settle(owner);
    owner = { settled: false, token: '', source: null, request: null, timer: null, failures: 0 };
    return owner;
  }

  function schedule(run, delay) {
    if (!admit(run) || run.request) return;
    clearTimer(run);
    run.timer = setTimeout(function () {
      run.timer = null;
      observe(run);
    }, delay);
  }

  function sourceObservation(run, source) {
    if (!admit(run, null, null, source)) return false;
    // New SSE makes both late successes AND errors of the old read obsolete.
    cancelRead(run);
    run.failures = 0;
    schedule(run, streamSilenceMs);
    return true;
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
    freshNotice = true;
    showStatus(notice.message);
  }

  // Watchdog, visibility and poll triggers coalesce onto this single read.
  // The deadline owns failure accounting even if abort cannot end a body read.
  function observe(run) {
    if (!admit(run) || run.request) return;
    clearTimer(run);
    var record = { controller: new AbortController(), deadline: null };
    run.request = record;
    function failure() {
      if (!admit(run, record)) return;
      cancelRead(run);
      run.failures++;
      if (run.failures >= maxStatusRetries) {
        settle(run);
        clearProgress();
        showStatus(unresolvedCopy);
      } else {
        // Preserve meaningful budget/provider status while retrying.
        schedule(run, statusRetryMs);
      }
    }
    record.deadline = setTimeout(failure, statusDeadlineMs);
    fetch('/api/rerate/status?surface=' + encodeURIComponent(surface), {
      headers: { 'Accept': 'application/json' }, cache: 'no-store', signal: record.controller.signal
    }).then(function (response) {
      if (!admit(run, record)) return null;
      if (!response.ok) throw new Error('status ' + response.status);
      return response.json();
    }).then(function (status) {
      if (!admit(run, record)) return;
      if (!status || ['idle', 'running', 'done', 'failed'].indexOf(status.state) === -1) {
        failure();
        return;
      }
      // Idle normally has no identity. If it does, it must pass the SAME
      // identity gate as every running/terminal observation before effects.
      if ((status.state !== 'idle' || status.run_token) && !admit(run, record, status)) return;
      cancelRead(run);
      run.failures = 0;
      if (status.state === 'running') {
        setRunning(true);
        showStatus(status.status || estimateCopy);
        showProgress(status.progress || '공고 분석을 준비하는 중...');
        schedule(run, run.source ? streamSilenceMs : 750);
        return;
      }
      settle(run);
      clearProgress();
      if (status.state === 'done' && !isHandled(run.token)) {
        var message = status.outcome === 'changed' ? completedAwayCopy : status.message;
        rememberAndReload(message || completedAwayCopy, run.token, entryToken);
      } else if (status.state === 'failed' && !isHandled(run.token)) {
        markHandled(run.token);
        showStatus(status.message || 'AI 평가에 실패했어요.');
      } else if (!freshNotice) {
        clearStatus();
      }
    }).catch(failure);
  }

  btn.addEventListener('click', function () {
    var run = beginOwner();
    freshNotice = false;
    log.textContent = '';
    setRunning(true);
    showStatus(estimateCopy);
    var source = new EventSource('/api/rerate?surface=' + encodeURIComponent(surface) +
      '&entry=' + encodeURIComponent(entryToken));
    run.source = source;
    schedule(run, streamSilenceMs);
    source.addEventListener('run-token', function (event) {
      if (!admit(run, null, { run_token: event.data, owner_entry: entryToken }, source)) return;
      sourceObservation(run, source);
    });
    source.addEventListener('status', function (event) {
      if (!sourceObservation(run, source)) return;
      showStatus(event.data);
    });
    source.addEventListener('progress', function (event) {
      if (!sourceObservation(run, source)) return;
      showProgress(event.data);
    });
    source.addEventListener('done', function (event) {
      if (!admit(run, null, null, source)) return;
      settle(run);
      clearProgress();
      rememberAndReload(event.data, run.token, entryToken);
    });
    source.addEventListener('failed', function (event) {
      if (!admit(run, null, null, source)) return;
      settle(run);
      clearProgress();
      markHandled(run.token);
      showStatus(event.data || 'AI 평가에 실패했어요.');
    });
    source.addEventListener('error', function () {
      if (!admit(run, null, null, source)) return;
      // Hidden tab: EventSource errors fire spuriously while backgrounded.
      // The stream is closed (no auto-reconnect → no second run); recovery
      // resumes via visibilitychange when the user returns.
      source.close();
      run.source = null;
      clearTimer(run);
      cancelRead(run);
      if (document.visibilityState === 'hidden') return;
      // The run may still be active server-side (detached, S8). Close the
      // stream — closing prevents the browser's automatic EventSource
      // reconnect from ever issuing a SECOND run — and adopt the active run
      // through the status endpoint instead.
      observe(run);
    });
  });

  window.addEventListener('pagehide', function () {
    if (owner) settle(owner);
    owner = null;
  });
  document.addEventListener('visibilitychange', function () {
    if (document.visibilityState !== 'visible') return;
    if (owner) observe(owner);
  });
  window.addEventListener('pageshow', function (event) {
    showStoredNotice();
    observe(owner || beginOwner());
  });
  showStoredNotice();
})();
