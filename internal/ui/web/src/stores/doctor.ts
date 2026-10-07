import { apiFetch } from '$lib/api';
import { readSSE } from '$lib/sse';

// Mirrors ui.DoctorCheck on the Go side. `fix`, when set, names a command from
// the site's command set (loadCommands) that resolves the finding.
export interface DoctorCheck {
  name: string;
  label?: string;
  status: 'ok' | 'warn' | 'fail' | 'unknown';
  detail?: string;
  fix?: string;
}

export interface DoctorReport {
  checks: DoctorCheck[];
  failures: number;
  warnings: number;
}

// loadDoctor streams the site's checks, handing each to onCheck the moment the
// backend finishes it, and resolves with the final report the panel settles on.
export async function loadDoctor(
  domain: string,
  branch = '',
  onCheck?: (check: DoctorCheck) => void
): Promise<DoctorReport> {
  const qs = new URLSearchParams({ stream: '1' });
  if (branch) qs.set('branch', branch);
  const res = await apiFetch(`/api/sites/${encodeURIComponent(domain)}/doctor?${qs}`);
  // The route returns 200 with an { error } body for refusals (unknown
  // worktree branch, site not found) instead of a stream, so surface it
  // rather than rendering an empty "all clear".
  if (!(res.headers.get('Content-Type') ?? '').startsWith('text/event-stream')) {
    const data = (await res.json().catch(() => ({}))) as { error?: string };
    throw new Error(data.error ?? `${res.status} ${res.statusText}`);
  }
  let report: DoctorReport | null = null;
  await readSSE(res, (event, data) => {
    if (event === 'check') onCheck?.(JSON.parse(data) as DoctorCheck);
    else if (event === 'done') report = normaliseReport(JSON.parse(data));
  });
  if (!report) throw new Error('doctor stream ended without a report');
  return report;
}

function normaliseReport(data: Partial<DoctorReport>): DoctorReport {
  return {
    checks: Array.isArray(data.checks) ? data.checks : [],
    failures: data.failures ?? 0,
    warnings: data.warnings ?? 0
  };
}
