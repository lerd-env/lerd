import { apiFetch } from './api';

// openInEditor asks lerd-ui to open a file at a line in the user's editor.
// Best-effort: lerd-ui resolves the editor (configured or autodetected) and
// execs it on the host. Loopback-only on the backend, so it's a no-op when the
// dashboard is opened from another machine.
export async function openInEditor(path: string, line: number): Promise<void> {
  if (!path) return;
  try {
    await apiFetch('/api/open-editor', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path, line })
    });
  } catch {
    // editor not found / not local — silently ignore.
  }
}
