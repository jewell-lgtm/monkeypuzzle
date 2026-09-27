// Only aggregate page usage: never repository names, titles, or OAuth parameters.
window.sanitizeAnalytics = function (_type, payload) {
  function clean(raw) {
    if (!raw) return '';
    try {
      const url = new URL(raw, window.location.origin);
      if (url.origin !== window.location.origin) return url.origin;
      if (url.pathname.startsWith('/repos/')) return '/repos/:owner/:name';
      return url.pathname;
    } catch { return ''; }
  }
  return { ...payload, url: clean(payload.url), referrer: clean(payload.referrer), title: 'Monkeypuzzle' };
};
