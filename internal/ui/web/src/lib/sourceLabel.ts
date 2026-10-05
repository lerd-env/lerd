// How a source location reads in the Debug window: from the project's root
// rather than the filesystem's, or by the class the call was made in.

export interface Frame {
  file?: string;
  line?: number;
  func?: string;
}

// relativeTo strips the deepest root that holds file, keeping it whole when
// none does.
export function relativeTo(file: string, roots: string[]): string {
  let best = '';
  for (const r of roots) {
    const root = r.endsWith('/') ? r : r + '/';
    if (file.startsWith(root) && root.length > best.length) best = root;
  }
  return best ? file.slice(best.length) : file;
}

// callerClass names the class the call at file:line was made from: the frame
// after it in the trace is the function that line belongs to, and when that is
// a method its class comes before the -> or ::. A closure is left to its file,
// since the class it is bound to (a route file's registrar, say) is not where
// it was written.
export function callerClass(file: string, line: number | undefined, trace: Frame[] | undefined): string {
  if (!trace) return '';
  const i = trace.findIndex((f) => f.file === file && f.line === line);
  const func = i >= 0 ? (trace[i + 1]?.func ?? '') : '';
  const m = /^([A-Za-z_\\][\w\\]*)(?:->|::)(?!\{closure)/.exec(func);
  return m ? m[1] : '';
}

// callerIndex is the frame a trace opens on: file:line, the first app frame,
// or else the first frame outside vendor/, and the innermost without one.
export function callerIndex(trace: Frame[], file: string, line: number | undefined): number {
  let i = trace.findIndex((f) => f.file === file && f.line === line);
  if (i < 0) i = trace.findIndex((f) => f.file && !f.file.includes('/vendor/'));
  return Math.max(i, 0);
}
