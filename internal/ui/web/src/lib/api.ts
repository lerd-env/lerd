export const apiBase =
  typeof location !== 'undefined' && location.hostname === 'lerd.localhost'
    ? 'http://localhost:7073'
    : '';

export function apiUrl(path: string): string {
  if (path.startsWith('http://') || path.startsWith('https://')) return path;
  return apiBase + path;
}

// CSRF header injected on every mutating call. The value is irrelevant —
// the server only checks that the header is present, because a cross-origin
// no-cors POST from a malicious page can't set a custom header without a
// CORS preflight that our origin allowlist would reject. See
// internal/ui/remote_control.go:csrfHeader.
const CSRF_HEADER = 'X-Lerd-CSRF';

const MUTATING_METHODS = new Set(['POST', 'PUT', 'DELETE', 'PATCH']);

function withCSRFHeader(init?: RequestInit): RequestInit | undefined {
  const method = (init?.method ?? 'GET').toUpperCase();
  if (!MUTATING_METHODS.has(method)) return init;
  const headers = new Headers(init?.headers);
  if (!headers.has(CSRF_HEADER)) headers.set(CSRF_HEADER, '1');
  return { ...(init ?? {}), headers };
}

export async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  return fetch(apiUrl(path), withCSRFHeader(init));
}

export async function apiJson<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await apiFetch(path, init);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json() as Promise<T>;
}

export function wsUrl(path: string): string {
  const u = new URL(apiUrl(path), location.href);
  u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:';
  return u.toString();
}
