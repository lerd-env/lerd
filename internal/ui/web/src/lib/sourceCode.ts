import { get } from 'svelte/store';
import { apiJson } from './api';
import { loadMonaco, lerdThemeName } from './monaco';
import { theme } from '$stores/theme';

// The lines around a frame's line, read from the site by lerd-ui and coloured
// by Monaco, which the dashboard already ships for tinker.

export interface SourceLine {
  n: number;
  text: string;
  html?: string;
}

const cache = new Map<string, Promise<SourceLine[]>>();

export function loadSource(file: string, line: number): Promise<SourceLine[]> {
  const key = `${file}:${line}`;
  let p = cache.get(key);
  if (!p) {
    p = apiJson<SourceLine[]>(`/api/source?file=${encodeURIComponent(file)}&line=${line}`).then((lines) => (/\.(php|phtml|inc)$/.test(file) ? colorize(lines) : lines));
    p.catch(() => cache.delete(key));
    cache.set(key, p);
  }
  return p;
}

// colorize keeps the plain text when Monaco cannot load, so the code still
// shows, uncoloured.
async function colorize(lines: SourceLine[]): Promise<SourceLine[]> {
  try {
    const monaco = await loadMonaco();
    monaco.editor.setTheme(lerdThemeName(get(theme)));
    const html = await monaco.editor.colorize(lines.map((l) => l.text).join('\n'), 'php', { tabSize: 4 });
    const parts = html.replace(/^<div[^>]*>|<\/div>$/g, '').split('<br/>');
    return lines.map((l, i) => ({ ...l, html: parts[i] }));
  } catch {
    return lines;
  }
}
