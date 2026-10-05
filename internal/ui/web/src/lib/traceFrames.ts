import type { Frame } from './sourceLabel';

// How a stack trace reads in the Debug window: each line named by the function
// it sits in, closures by where they were written, vendor runs grouped.

export interface TraceFrame extends Frame {
  pkg?: string;
}

export interface TraceRow {
  i: number;
  file: string;
  line: number;
  // inside is the function the line sits in: PHP names each frame by the
  // function called at its line, so it is the next frame's name.
  inside: string;
  called: string;
  pkg: string;
  app: boolean;
}

export interface TraceRun {
  app: boolean;
  rows: TraceRow[];
}

// The package a frame belongs to: the one Composer recorded for its path,
// or else the vendor/<vendor>/<name> it sits in.
export function packageOf(f: TraceFrame): string {
  if (f.pkg) return f.pkg;
  const m = /\/vendor\/([^/]+\/[^/]+)\//.exec(f.file ?? '');
  return m ? m[1] : '';
}

export function traceRows(trace: TraceFrame[]): TraceRow[] {
  return trace.map((f, i) => {
    const pkg = packageOf(f);
    return { i, file: f.file ?? '', line: f.line ?? 0, inside: trace[i + 1]?.func ?? '', called: f.func ?? '', pkg, app: !pkg };
  });
}

// traceRuns keeps each app frame on its own and folds consecutive vendor
// frames into one run.
export function traceRuns(rows: TraceRow[]): TraceRun[] {
  const out: TraceRun[] = [];
  for (const r of rows) {
    const last = out[out.length - 1];
    if (last && !r.app && !last.app) last.rows.push(r);
    else out.push({ app: r.app, rows: [r] });
  }
  return out;
}

const base = (path: string) => path.split('/').pop() ?? path;

// funcLabel reads a PHP frame name the way it was written: Class->method(),
// a closure by the method or file it was declared in, the namespace left to
// the full name.
export function funcLabel(func: string): string {
  if (!func) return '{main}';
  const closure = /\{closure:(?:\{closure:)?([^{}]*?)(?:\}:\d+)?\}$/.exec(func);
  if (closure) {
    const inner = closure[1];
    const method = /^(?:.*\\)?(\w+)(?:::|->)(\w+)\(\):\d+$/.exec(inner);
    if (method) return `closure in ${method[1]}::${method[2]}()`;
    const file = /^(.+\.php):(\d+)$/.exec(inner);
    if (file) return `closure in ${base(file[1])}:${file[2]}`;
    return 'closure';
  }
  const m = /^(?:.*\\)?(\w+)(->|::)(.+)$/.exec(func);
  if (m) return `${m[1]}${m[2]}${m[3]}()`;
  return `${func.split('\\').pop()}()`;
}

// traceText is the trace in PHP's own #0 file(line): function() form.
export function traceText(trace: TraceFrame[]): string {
  return trace.map((f, i) => `#${i} ${f.file ?? '[internal]'}(${f.line ?? 0}): ${f.func || '{main}'}()`).join('\n');
}

// Whether trace views show the code beside the path, remembered per browser.
const CODE_KEY = 'lerd:trace-code';
export function traceCodePref(): boolean {
  try {
    return localStorage.getItem(CODE_KEY) !== 'off';
  } catch {
    return true;
  }
}
export function saveTraceCodePref(on: boolean) {
  try {
    localStorage.setItem(CODE_KEY, on ? 'on' : 'off');
  } catch {}
}
