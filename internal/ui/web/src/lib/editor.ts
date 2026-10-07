import { apiFetch } from './api';

// openInEditor asks lerd-ui to open a file at a line in the host's editor.
// The backend requires dashboard-control authority and confines paths to the
// user's home directory.
export async function openInEditor(path: string, line: number): Promise<void> {
  if (!path) return;
  try {
    const res = await apiFetch('/api/open-editor', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path, line })
    });
    // An editor reached by its URL scheme rather than its binary is opened by
    // the browser handing the URL to the desktop.
    if (res.status === 200) {
      const { url } = (await res.json()) as { url?: string };
      if (url) window.location.href = url;
    }
  } catch {
    // editor not found / not local — silently ignore.
  }
}
