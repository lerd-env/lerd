import { parseDump, looksLikeDump, type DumpNode } from './dump-parser';

// toDumpNodes turns a captured value into the tree DumpView navigates: text a
// VarDumper rendered, a JSON string, or a plain object or array. Anything else
// (a scalar, prose) yields null and is shown as text.
export function toDumpNodes(value: unknown): DumpNode[] | null {
  if (typeof value === 'string') {
    const t = value.trim();
    if (looksLikeDump(t)) {
      const r = parseDump(t);
      return r.ok ? r.nodes : null;
    }
    if ((t.startsWith('{') && t.endsWith('}')) || (t.startsWith('[') && t.endsWith(']'))) {
      try {
        return [fromJson(JSON.parse(t))];
      } catch {
        return null;
      }
    }
    return null;
  }
  return value !== null && typeof value === 'object' ? [fromJson(value)] : null;
}

function fromJson(v: unknown): DumpNode {
  if (Array.isArray(v)) return { kind: 'array', count: v.length, items: v.map((x, i) => ({ key: String(i), value: fromJson(x) })) };
  if (v !== null && typeof v === 'object') {
    const entries = Object.entries(v as Record<string, unknown>);
    return { kind: 'array', count: entries.length, items: entries.map(([k, x]) => ({ key: JSON.stringify(k), value: fromJson(x) })) };
  }
  if (typeof v === 'string') return { kind: 'scalar', type: 'string', value: JSON.stringify(v) };
  if (typeof v === 'number') return { kind: 'scalar', type: 'number', value: String(v) };
  if (typeof v === 'boolean') return { kind: 'scalar', type: 'bool', value: String(v) };
  return { kind: 'scalar', type: 'null', value: 'null' };
}

// splitLogContext separates the JSON a log line formatter appends to a message
// (Monolog's "message {context} [extra]") from the message itself, so the
// context can be shown as a tree. A line without one comes back whole.
export function splitLogContext(line: string): { text: string; context?: unknown } {
  for (let i = line.indexOf(' ', 0); i >= 0; i = line.indexOf(' ', i + 1)) {
    const rest = line.slice(i + 1).trim();
    if (rest[0] !== '{' && rest[0] !== '[') continue;
    for (const candidate of [rest, rest.replace(/\s+(\[\]|\{.*\})$/, '')]) {
      try {
        const context = JSON.parse(candidate);
        if (context !== null && typeof context === 'object') return { text: line.slice(0, i).trimEnd(), context };
      } catch {
        /* not where the context starts */
      }
    }
  }
  return { text: line };
}
