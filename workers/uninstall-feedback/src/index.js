const MAX_BYTES = 4 * 1024;
const REASONS = new Set(['broken', 'heavy', 'switched', 'trying', 'other']);
const OSES = new Set(['linux', 'darwin', 'windows']);
const VERSION = /^\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$/;

const reply = (status) => new Response(null, { status });

// validate returns the stored shape, or null for anything a lerd binary would
// never send.
export function validate(input) {
  if (!input || typeof input !== 'object' || Array.isArray(input)) return null;
  const { reason, comment = '', version, os } = input;
  if (!REASONS.has(reason) || !OSES.has(os)) return null;
  if (typeof version !== 'string' || !VERSION.test(version)) return null;
  if (typeof comment !== 'string' || [...comment].length > 500) return null;
  return { reason, comment: comment.trim(), version, os };
}

export async function record(request, db) {
  const body = await request.text();
  if (new TextEncoder().encode(body).length > MAX_BYTES) return reply(413);

  let parsed;
  try {
    parsed = JSON.parse(body);
  } catch {
    return reply(400);
  }
  const clean = validate(parsed);
  if (!clean) return reply(400);

  await db
    .prepare(`INSERT INTO answers (day, reason, comment, version, os) VALUES (date('now'), ?1, ?2, ?3, ?4)`)
    .bind(clean.reason, clean.comment, clean.version, clean.os)
    .run();
  return reply(204);
}

export default {
  async fetch(request, env) {
    const { pathname } = new URL(request.url);
    if (pathname !== '/v1/uninstall') return reply(404);
    if (request.method !== 'POST') return reply(405);
    // The address only keys Cloudflare's counter; it is never stored.
    const { success } = await env.LIMIT.limit({ key: request.headers.get('cf-connecting-ip') ?? '' });
    if (!success) return reply(429);
    return record(request, env.DB);
  }
};
