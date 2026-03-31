// ══════════════════════════════════════
// Analytics — click tracking, session duration, page views, UTM source
// ══════════════════════════════════════
(function () {
  'use strict';

  var ENDPOINT = '/api/v1/analytics/events';
  var FLUSH_INTERVAL = 15000; // 15s
  var SESSION_KEY = 'if_analytics_sid';
  var NEW_VISITOR_KEY = 'if_analytics_visited';

  // ── Helpers ──────────────────────────────────────────────
  function getSessionId() {
    var sid = sessionStorage.getItem(SESSION_KEY);
    if (!sid) {
      sid = Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 10);
      sessionStorage.setItem(SESSION_KEY, sid);
    }
    return sid;
  }

  function isNewVisitor() {
    if (localStorage.getItem(NEW_VISITOR_KEY)) return false;
    localStorage.setItem(NEW_VISITOR_KEY, '1');
    return true;
  }

  function getUtmParams() {
    var params = new URLSearchParams(location.search);
    return {
      utm_source:   params.get('utm_source')   || '',
      utm_medium:   params.get('utm_medium')   || '',
      utm_campaign: params.get('utm_campaign') || '',
    };
  }

  // ── State ─────────────────────────────────────────────────
  var sid = getSessionId();
  var queue = [];
  var sessionStart = Date.now();
  var currentPage = location.hash ? location.hash.replace('#', '/') : '/';
  var pageEnterTime = Date.now();

  // ── Queue helpers ─────────────────────────────────────────
  function track(type, extra) {
    var ev = { session_id: sid, type: type, page: currentPage };
    if (extra) {
      for (var k in extra) { ev[k] = extra[k]; }
    }
    queue.push(ev);
  }

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

  // ── Page view tracking ────────────────────────────────────
  function trackPageView(page, durationMs) {
    track('page_view', { page: page, duration_page_ms: durationMs || 0 });
  }

  function onPageChange(newPage) {
    var elapsed = Date.now() - pageEnterTime;
    trackPageView(currentPage, elapsed);
    currentPage = newPage;
    pageEnterTime = Date.now();
    flush();
  }

  // ── Session start ─────────────────────────────────────────
  var utm = getUtmParams();
  track('session_start', {
    referrer: document.referrer || '',
    is_new: isNewVisitor(),
    utm_source:   utm.utm_source,
    utm_medium:   utm.utm_medium,
    utm_campaign: utm.utm_campaign,
  });
  // First page view (enter)
  trackPageView(currentPage, 0);

  // ── Click tracking ────────────────────────────────────────
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

  // ── SPA hash-based page changes ───────────────────────────
  window.addEventListener('hashchange', function () {
    var newPage = location.hash ? location.hash.replace('#', '/') : '/';
    onPageChange(newPage);
  });

  // ── Periodic flush ────────────────────────────────────────
  setInterval(flush, FLUSH_INTERVAL);

  // ── Session end on page unload ────────────────────────────
  function endSession() {
    var elapsed = Date.now() - pageEnterTime;
    trackPageView(currentPage, elapsed);
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

