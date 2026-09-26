import { derived, get, writable } from 'svelte/store';
import { apiFetch, apiJson, decodeJSONResult } from '$lib/api';
import { readSSE } from '$lib/sse';
import { notifyLocalInfo } from '$lib/notify';
import { wizardState } from '$lib/wizardState';
import { modal } from '$stores/modals';
import { m } from '../paraglide/messages.js';

export interface FrameworkChoice {
  name: string;
  label: string;
  versions?: string[];
  latest?: string;
}

export interface ChoiceOption {
  value: string;
  label: string;
}

// ProjectQuestions mirrors what `lerd init` asks about a directory. The server
// decides what there is to ask; the wizard only renders it.
export interface ProjectQuestions {
  dir: string;
  kind: string;
  kind_choice: boolean;
  kind_options?: ChoiceOption[];
  kind_title?: string;
  framework?: string;
  framework_label?: string;
  php_version?: string;
  php_installed?: string[];
  node_managed: boolean;
  node_version?: string;
  node_installed?: string[];
  node_version_of?: string;
  https_available: boolean;
  secured: boolean;
  database_options?: ChoiceOption[];
  database?: string;
  service_options?: string[];
  services?: string[];
  frankenphp_offered: boolean;
  frankenphp_reason?: string;
  frankenphp: boolean;
  frankenphp_worker: boolean;
  worker_options?: string[];
  workers?: string[];
  proxy_command?: string;
  proxy_command_hint?: string;
  proxy_port?: number;
  proxy_vite_pitfall?: boolean;
  proxy_rails_pitfall?: boolean;
  container_port?: number;
  containerfile?: string;
}

export interface ProjectAnswers {
  kind: string;
  php_version?: string;
  node_version?: string;
  secured: boolean;
  database?: string;
  services: string[];
  frankenphp?: boolean;
  frankenphp_worker?: boolean;
  workers: string[];
  proxy_command?: string;
  proxy_port?: number;
  container_port?: number;
  containerfile?: string;
}

export interface SetupStep {
  label: string;
  enabled: boolean;
  optional: boolean;
}

export interface RunSnapshot {
  id: string;
  kind: string;
  dir: string;
  label?: string;
  status: 'running' | 'done' | 'failed';
  error?: string;
  started: number;
}

export interface RunRequest {
  kind: 'scaffold' | 'link' | 'env' | 'setup';
  dir: string;
  name?: string;
  framework?: string;
  framework_version?: string;
  steps?: string[];
}

export interface RunEvent {
  line?: string;
  done?: boolean;
  ok?: boolean;
  error?: string;
}

export async function frameworkCatalogue(): Promise<FrameworkChoice[]> {
  const res = await apiJson<{ frameworks?: FrameworkChoice[] }>('/api/frameworks/catalogue');
  return res.frameworks ?? [];
}

export async function projectQuestions(dir: string): Promise<ProjectQuestions> {
  const res = await apiJson<ProjectQuestions & { error?: string }>(
    '/api/project/questions?dir=' + encodeURIComponent(dir)
  );
  if (res.error) throw new Error(res.error);
  return res;
}

export async function saveProjectAnswers(dir: string, answers: ProjectAnswers): Promise<void> {
  const res = await apiFetch('/api/project/questions?dir=' + encodeURIComponent(dir), {
    method: 'POST',
    body: JSON.stringify(answers)
  });
  const out = await decodeJSONResult<{ ok?: boolean; error?: string }>(res);
  if (out.error || !out.ok) throw new Error(out.error || 'saving the answers failed');
}

export async function setupSteps(dir: string): Promise<SetupStep[]> {
  const res = await apiJson<{ steps?: SetupStep[]; error?: string }>(
    '/api/project/setup-steps?dir=' + encodeURIComponent(dir)
  );
  if (res.error) throw new Error(res.error);
  return res.steps ?? [];
}

// startRun asks lerd-ui to run one named piece of work on the host. The run
// keeps going after this page closes, so what comes back is the handle to
// reattach to rather than the work itself.
export async function startRun(req: RunRequest): Promise<RunSnapshot> {
  const res = await apiFetch('/api/runs', { method: 'POST', body: JSON.stringify(req) });
  const out = await decodeJSONResult<{ run?: RunSnapshot; error?: string }>(res);
  if (out.error || !out.run) throw new Error(out.error || 'starting the run failed');
  return out.run;
}

export async function runsForDir(dir: string): Promise<RunSnapshot[]> {
  const res = await apiJson<{ runs?: RunSnapshot[] }>('/api/runs?dir=' + encodeURIComponent(dir));
  return res.runs ?? [];
}

// The run the wizard left going on the host, if any. Sending it to the
// background closes the modal, so this is what keeps the work visible: the
// button that reopens the wizard spins while it is set.
export const activeRun = writable<RunSnapshot | null>(null);

let watchTimer: ReturnType<typeof setTimeout> | undefined;
let watched: string | null = null;

// projectOf names the directory a run is about, for a message that says which
// project finished rather than which command did.
function projectOf(run: RunSnapshot): string {
  const path = run.label || run.dir;
  return path.split('/').filter(Boolean).pop() ?? path;
}

// announce reports a run that ended while nobody was watching it. The wizard
// reports its own runs on screen, so this only speaks when the modal is closed,
// which is exactly when the work was sent to the background.
//
// The daemon raises the ones worth interrupting for, a finished scaffold and any
// failure, and those reach this page over the websocket like every other
// notification. This covers what it deliberately stays quiet about: the quick
// steps, whose ending is still news to someone who walked away from them.
function announce(run: RunSnapshot) {
  if (get(modal).kind === 'link') return;
  if (run.status === 'failed' || run.kind === 'scaffold') return;
  notifyLocalInfo(
    'op_done',
    m.siteWizard_notifyDoneTitle(),
    m.siteWizard_notifyDoneBody({ project: projectOf(run) })
  );
}

export async function refreshActiveRun(): Promise<RunSnapshot | null> {
  try {
    const all = await runsForDir('');
    const running = all.find((r) => r.status === 'running') ?? null;
    if (watched && running?.id !== watched) {
      const ended = all.find((r) => r.id === watched);
      if (ended && ended.status !== 'running') announce(ended);
    }
    watched = running?.id ?? null;
    activeRun.set(running);
    return running;
  } catch {
    return null;
  }
}

// watchActiveRun polls only while something is running, so a finished run stops
// the spinner without the dashboard checking forever on an idle machine.
// wizardBubble is the parked flow as the bubble draws it: what is running, or
// what is waiting to be picked back up. Null while the wizard is on screen,
// which is where the same state is already visible.
export const wizardBubble = derived(
  [wizardState, activeRun, modal],
  ([$state, $run, $modal]) => {
    if (!$state || $state.step === 'start' || $modal.kind === 'link') return null;
    const path = $state.dir || $state.parent || '';
    return {
      project: path.split('/').filter(Boolean).pop() ?? path,
      running: Boolean($run),
      label: $run?.label || ''
    };
  }
);

export function watchActiveRun(intervalMs = 4000): void {
  clearTimeout(watchTimer);
  const tick = async () => {
    const running = await refreshActiveRun();
    if (running) watchTimer = setTimeout(tick, intervalMs);
  };
  void tick();
}

// streamRun replays what a run has printed so far and then follows it. Calling
// it again on the same run is how a reloaded page picks up mid-composer.
export async function streamRun(id: string, onEvent: (e: RunEvent) => void): Promise<void> {
  const res = await apiFetch('/api/runs/' + encodeURIComponent(id) + '/stream');
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  await readSSE(res, (event, data) => {
    if (event !== 'done') {
      onEvent({ line: data });
      return;
    }
    try {
      const result = JSON.parse(data) as { ok?: boolean; error?: string };
      onEvent({ done: true, ok: Boolean(result.ok), error: result.error });
    } catch {
      onEvent({ done: true, ok: false, error: 'bad done payload' });
    }
  });
}
