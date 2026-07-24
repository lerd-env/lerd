import { apiFetch } from '$lib/api';
import { readSSE } from '$lib/sse';

export interface LinkEvent {
  line?: string;
  done?: boolean;
  ok?: boolean;
  domain?: string;
  error?: string;
  // Set when the site linked but its environment setup did not, so the modal
  // can say so instead of reporting a clean success.
  warning?: string;
}

export async function streamLinkSite(path: string, onEvent: (e: LinkEvent) => void): Promise<void> {
  const res = await apiFetch('/api/sites/link?path=' + encodeURIComponent(path), { method: 'POST' });
  await readSSE(res, (event, data) => {
    if (event === 'done') {
      try {
        const result = JSON.parse(data) as {
          ok?: boolean;
          domain?: string;
          error?: string;
          warning?: string;
        };
        onEvent({
          done: true,
          ok: Boolean(result.ok),
          domain: result.domain,
          error: result.error,
          warning: result.warning,
        });
      } catch {
        onEvent({ done: true, ok: false, error: 'bad done payload' });
      }
    } else {
      onEvent({ line: data });
    }
  });
}
