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
  var lifecycleGeneration = 0;
  var activeRunToken = '';
  // Stream-silence watchdog + bounded recovery-retry state.
  var silenceTimer = null;
  var lastStreamEventAt = 0;
  var statusFailures = 0;
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
  // Observation fence: bumped by every newer observation (stream event,
  // adopted status response, lifecycle cancellation). A status response whose
  // request epoch no longer matches is STALE — a newer SSE/status observation
  // already spoke — and is dropped instead of overwriting it. This also
  // serializes concurrent probes: only the first response to arrive may adopt.
  var statusEpoch = 0;
  var statusInflight = [];

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

  // cancelPendingStatus invalidates every outstanding status request at once:
  // it bumps the observation fence (late responses become stale and are
  // dropped) and aborts the underlying fetches with their deadline timers. A
  // terminal adoption, exhaustion, or lifecycle stop can never be resurrected
  // or clobbered by a response that was already on the wire.
  function cancelPendingStatus() {
    statusEpoch++;
    for (var i = 0; i < statusInflight.length; i++) {
      var record = statusInflight[i];
      if (record.deadline) {
        clearTimeout(record.deadline);
        record.deadline = null;
      }
      record.controller.abort();
    }
    statusInflight = [];
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
    if (silenceTimer) {
      clearTimeout(silenceTimer);
      silenceTimer = null;
    }
    cancelPendingStatus();
    return lifecycleGeneration;
  }

  // stopStream closes only the SSE stream, keeping any status polling alive —
  // used when the stream errors while the detached run may still be active.
  function stopStream() {
    lifecycleGeneration++;
    if (eventSource) {
      eventSource.close();
      eventSource = null;
    }
    if (silenceTimer) {
      clearTimeout(silenceTimer);
      silenceTimer = null;
    }
    cancelPendingStatus();
    return lifecycleGeneration;
  }

  // noteStreamEvent records stream liveness and re-arms the silence watchdog:
  // while the OWNED stream keeps delivering, no status probe is spent.
  function noteStreamEvent() {
    lastStreamEventAt = Date.now();
    if (silenceTimer) clearTimeout(silenceTimer);
    if (!eventSource) return;
    silenceTimer = setTimeout(checkSilentStream, streamSilenceMs);
  }

  // checkSilentStream is the bounded status-only recovery for a stream that
  // stays OPEN but silent (proxy stall, dropped middle): one probe of the
  // existing status endpoint — never a new EventSource, so a recovering
  // client can never start a second provider run. The healthy stream is left
  // open; the watchdog re-arms after the probe completes.
  function checkSilentStream() {
    silenceTimer = null;
    if (!eventSource) return;
    pollStatus(lifecycleGeneration, true);
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

  // pollStatus reads the existing status endpoint. In probe mode (silent
  // stream check) it never touches the running UI beyond adopting the owned
  // run's latest copy, and never cancels the still-open stream; in full mode
  // it owns the recovery loop. Both the fetch and the body read carry a
  // deadline; expiry (a hung response) enters the SAME bounded retry path as
  // a rejection, and repeated failure stops at maxStatusRetries with an
  // explicit unresolved state — never an infinite loop, never a dead page.
  // Every response is fenced: it is adopted only when no newer stream/status
  // observation happened while it was in flight (request epoch match).
  function pollStatus(generation, probe) {
    if (!isCurrent(generation)) return;
    if (!probe) {
      if (pollTimer) {
        clearTimeout(pollTimer);
        pollTimer = null;
      }
      cancelPendingStatus();
    }
    var requestEpoch = statusEpoch;
    var controller = new AbortController();
    var record = { controller: controller, deadline: null, timedOut: false };
    statusInflight.push(record);
    record.deadline = setTimeout(function () {
      record.timedOut = true;
      record.deadline = null;
      controller.abort();
    }, statusDeadlineMs);
    function settleCleanup() {
      if (record.deadline) {
        clearTimeout(record.deadline);
        record.deadline = null;
      }
      var index = statusInflight.indexOf(record);
      if (index !== -1) statusInflight.splice(index, 1);
    }
    fetch('/api/rerate/status?surface=' + encodeURIComponent(surface), {
      headers: { 'Accept': 'application/json' },
      cache: 'no-store',
      signal: controller.signal
    }).then(function (response) {
      if (!isCurrent(generation)) return null;
      if (!response.ok) throw new Error('status ' + response.status);
      return response.json();
    }).then(function (status) {
      settleCleanup();
      if (!isCurrent(generation)) return;
      if (!status) return;
      statusFailures = 0;
      // Stale response: a newer observation (stream event or another
      // adoption) already spoke while this one was in flight — drop it so a
      // late snapshot can never regress newer progress.
      if (statusEpoch !== requestEpoch) return;
      if (probe && eventSource) {
        // Silent-stream probe outcome: adopt the owned run's live copy. The
        // watchdog re-arms ONLY for an owned active run — a foreign or
        // terminal state terminates the silence timer (no polling another
        // entry's run; a later live stream event re-arms on its own).
        if (adoptStatus(status, generation)) noteStreamEvent();
        return;
      }
      var adopted = adoptStatus(status, generation);
      if (adopted && status.state === 'running') {
        pollTimer = setTimeout(function () {
          if (!isCurrent(generation)) return;
          pollTimer = null;
          pollStatus(generation, false);
        }, 750);
      }
    }).catch(function (error) {
      settleCleanup();
      if (!isCurrent(generation)) return;
      // A deliberate lifecycle cancellation is silent; a deadline expiry
      // (record.timedOut) is a transport fault and must retry like any other.
      if (error && error.name === 'AbortError' && !record.timedOut) return;
      statusFailures++;
      if (statusFailures >= maxStatusRetries) {
        // Bounded exhaustion: stop retrying, invalidate outstanding probes,
        // state the unresolved outcome, and hand control back to the user —
        // never claim completion.
        if (pollTimer) { clearTimeout(pollTimer); pollTimer = null; }
        if (silenceTimer) { clearTimeout(silenceTimer); silenceTimer = null; }
        cancelPendingStatus();
        setRunning(false);
        clearProgress();
        showStatus(unresolvedCopy);
        return;
      }
      if (probe && eventSource) {
        noteStreamEvent(); // stream still open: re-arm the watchdog
        return;
      }
      setRunning(true);
      showStatus(estimateCopy);
      pollTimer = setTimeout(function () {
        if (!isCurrent(generation)) return;
        pollTimer = null;
        pollStatus(generation, false);
      }, statusRetryMs);
    });
  }

  // adoptStatus applies one status snapshot to the page under the existing
  // ownership/handled rules — shared by the recovery poll and the probe. It
  // returns true only when it adopted an OWNED, still-running run (terminal
  // states return true after ending the loop). Adopting makes this response
  // the newest observation; a terminal adoption also closes the still-open
  // stream and cancels every outstanding probe, so nothing later can revive
  // the ended run or clear its notice.
  function adoptStatus(status, generation) {
    var handled = isHandled(status.run_token);
    if (status.state === 'running') {
      if (!ownsStatus(status)) {
        setRunning(false);
        clearStatus();
        clearProgress();
        return false;
      }
      // Identity fence: this entry may own a NEWER run than the one this page
      // pressed (another tab pressed again). A running snapshot for a
      // different run_token never overwrites the known active run's state and
      // never polls it. When the known stream is still live that is enough;
      // when recovery owns the loop (stream dead) the known run is gone
      // server-side, so the loop ends terminally-safe: loading stops, the
      // still-honest progress stays on screen, and a hint tells the user the
      // run moved. An unknown activeRunToken (fresh page adopting after
      // reload) still adopts freely.
      if (activeRunToken && status.run_token && status.run_token !== activeRunToken) {
        if (eventSource) return false;
        if (pollTimer) { clearTimeout(pollTimer); pollTimer = null; }
        cancelPendingStatus();
        setRunning(false);
        showStatus('이전 요청이 종료됐어요. 최근 상태는 새로고침하면 확인할 수 있어요.');
        return false;
      }
      statusEpoch++;
      setRunning(true);
      showStatus(status.status || estimateCopy);
      showProgress(status.progress || '공고 분석을 준비하는 중...');
      return true;
    }

    setRunning(false);
    clearProgress();
    if (status.state === 'idle') {
      clearStatus();
      return true;
    }
    if (!ownsStatus(status)) {
      clearStatus();
      return true;
    }
    if (status.state === 'done') {
      if (!handled) {
        stopTransport();
        var message = status.outcome === 'changed' ? completedAwayCopy : status.message;
        rememberAndReload(message || completedAwayCopy, status.run_token, status.owner_entry);
        return true;
      }
      // done+handled after the completion reload: keep the fresh notice this
      // page just displayed (done+handled must not clobber it); otherwise the
      // run is old news on an unrelated page — clear it.
      stopTransport();
      if (!freshNotice) clearStatus();
      return true;
    }
    if (status.state === 'failed') {
      stopTransport();
      if (handled) {
        clearStatus();
        return true;
      }
      markHandled(status.run_token);
      showStatus(status.message || 'AI 평가에 실패했어요.');
      return true;
    }
    clearStatus();
    return true;
  }

  btn.addEventListener('click', function () {
    var generation = stopTransport();
    activeRunToken = '';
    statusFailures = 0;
    log.textContent = '';
    setRunning(true);
    showStatus(estimateCopy);
    var source = new EventSource('/api/rerate?surface=' + encodeURIComponent(surface) +
      '&entry=' + encodeURIComponent(entryToken));
    eventSource = source;
    noteStreamEvent();
    source.addEventListener('run-token', function (event) {
      if (!isCurrent(generation)) return;
      statusEpoch++;
      noteStreamEvent();
      activeRunToken = event.data || '';
    });
    source.addEventListener('status', function (event) {
      if (!isCurrent(generation)) return;
      statusEpoch++;
      noteStreamEvent();
      showStatus(event.data);
    });
    source.addEventListener('progress', function (event) {
      if (!isCurrent(generation)) return;
      statusEpoch++;
      noteStreamEvent();
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
      // Hidden tab: EventSource errors fire spuriously while backgrounded.
      // The stream is closed (no auto-reconnect → no second run); recovery
      // resumes via visibilitychange when the user returns.
      if (document.visibilityState === 'hidden') {
        stopStream();
        return;
      }
      // The run may still be active server-side (detached, S8). Close the
      // stream — closing prevents the browser's automatic EventSource
      // reconnect from ever issuing a SECOND run — and adopt the active run
      // through the status endpoint instead.
      var pollGeneration = stopStream();
      pollStatus(pollGeneration, false);
    });
  });

  window.addEventListener('pagehide', stopTransport);
  document.addEventListener('visibilitychange', function () {
    if (document.visibilityState !== 'visible') return;
    // Returning to a visible tab: probe once whether the owned run advanced
    // while hidden — the stream may be dead-but-non-null (silently dropped),
    // so recovery must not depend on eventSource being null.
    if (eventSource) {
      if (silenceTimer) clearTimeout(silenceTimer);
      pollStatus(lifecycleGeneration, true);
      return;
    }
    pollStatus(stopTransport());
  });
  window.addEventListener('pageshow', function (event) {
    showStoredNotice();
    pollStatus(stopTransport());
  });
  showStoredNotice();
})();
