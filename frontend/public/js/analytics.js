// ══════════════════════════════════════
// Analytics — click tracking & session duration
// ══════════════════════════════════════
(function () {
  'use strict';

  var ENDPOINT = '/api/v1/analytics/events';
  var FLUSH_INTERVAL = 15000; // 15s
  var SESSION_KEY = 'if_analytics_sid';

  // Generate or retrieve session ID
  function getSessionId() {
    var sid = sessionStorage.getItem(SESSION_KEY);
    if (!sid) {
      sid = Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 10);
      sessionStorage.setItem(SESSION_KEY, sid);
    }
    return sid;
  }

  var sid = getSessionId();
  var queue = [];
  var sessionStart = Date.now();
  var currentPage = location.hash ? location.hash.replace('#', '/') : '/';

  // Push event to queue
  function track(type, extra) {
    var ev = { session_id: sid, type: type, page: currentPage };
    if (extra) {
      for (var k in extra) { ev[k] = extra[k]; }
    }
    queue.push(ev);
  }

  // Flush queue to backend
  function flush() {
    if (!queue.length) return;
    var batch = queue.splice(0, 50);
    try {
      if (navigator.sendBeacon) {
        navigator.sendBeacon(ENDPOINT, JSON.stringify(batch));
      } else {
        var xhr = new XMLHttpRequest();
        xhr.open('POST', ENDPOINT, true);
        xhr.setRequestHeader('Content-Type', 'application/json');
        xhr.send(JSON.stringify(batch));
      }
    } catch (e) { /* silent */ }
  }

  // Session start
  track('session_start', { referrer: document.referrer || '' });

  // Click tracking
  document.addEventListener('click', function (e) {
    var el = e.target.closest('a, button, [onclick], .btn, .tkt-card, .nav-links a, .qnav-card');
    if (!el) return;
    var label = el.textContent ? el.textContent.trim().slice(0, 80) : '';
    var tag = el.tagName.toLowerCase();
    var id = el.id || '';
    var target = tag;
    if (id) target += '#' + id;
    if (label) target += ':' + label;
    track('click', { target: target });
  }, true);

  // Track SPA page changes via hash
  window.addEventListener('hashchange', function () {
    currentPage = location.hash ? location.hash.replace('#', '/') : '/';
  });

  // Periodic flush
  setInterval(flush, FLUSH_INTERVAL);

  // Session end on page unload
  function endSession() {
    var duration = Date.now() - sessionStart;
    track('session_end', { duration_ms: duration });
    flush();
  }

  window.addEventListener('beforeunload', endSession);
  document.addEventListener('visibilitychange', function () {
    if (document.visibilityState === 'hidden') {
      endSession();
    }
  });
})();
