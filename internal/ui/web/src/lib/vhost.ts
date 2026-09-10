// The dashboard has one canonical address, the nginx vhost. 127.0.0.1:7073 is
// only ever the fallback for a stopped stack, because nginx is a container
// `lerd stop` stops while lerd-ui keeps serving on its own port.
//
// Lingering on the fallback is what this module exists to prevent: browser
// permissions are per-origin, so a session that settles on the port would ask
// for notifications a second time and register a second push subscription,
// leaving two "browsers" in the notifications list delivering everything twice.
export const VHOST_URL = 'http://lerd.localhost';

// onFallbackOrigin reports whether this page is the fallback address. A LAN
// session is served on the same port from the host's own IP and lerd.localhost
// does not resolve there, so only loopback qualifies.
export function onFallbackOrigin(): boolean {
  return (
    typeof location !== 'undefined' &&
    location.port === '7073' &&
    (location.hostname === '127.0.0.1' || location.hostname === 'localhost')
  );
}

// handOverToVhost moves the session to the vhost as soon as it answers, and is
// a no-op anywhere else. The probe is no-cors because the body is not readable
// cross-origin: what matters is that nginx accepted the connection at all, so a
// vhost that is up but broken never strands the session on a dead page.
//
// The response type is what decides it. "Did not throw" is not enough: the
// service worker answers a failed request with a synthesized error Response, so
// a stopped nginx would read as a running one and hand the session to a page
// that cannot load.
export async function handOverToVhost(): Promise<void> {
  if (!onFallbackOrigin()) return;
  try {
    const res = await fetch(VHOST_URL + '/?lerd-probe=1', { mode: 'no-cors', cache: 'no-store' });
    if (res.type !== 'opaque' && !res.ok) return;
  } catch {
    return;
  }
  // The hash carries the route, and every notification deep link arrives on
  // this origin (the desktop app only ever loads the loopback address), so
  // dropping it here would land every click on the plain dashboard.
  location.href = VHOST_URL + '/' + location.hash;
}
