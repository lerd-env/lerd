import { writable } from 'svelte/store';
import { apiJson } from './api';

// The lines around a frame's line, read from the site by lerd-ui.

export interface SourceLine {
  n: number;
  text: string;
}

// sourceAvailable is false where lerd-ui cannot be asked for a site's code,
// the debug bar on a site reached from more than this machine, so trace views
// show the path alone.
export const sourceAvailable = writable(true);

const cache = new Map<string, Promise<SourceLine[]>>();

export function loadSource(file: string, line: number): Promise<SourceLine[]> {
  const key = `${file}:${line}`;
  let p = cache.get(key);
  if (!p) {
    p = apiJson<SourceLine[]>(`/api/source?file=${encodeURIComponent(file)}&line=${line}`);
    p.catch(() => cache.delete(key));
    cache.set(key, p);
  }
  return p;
}
