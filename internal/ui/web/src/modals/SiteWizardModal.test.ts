import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import SiteWizardModal from './SiteWizardModal.svelte';
import { sites } from '$stores/sites';

const projectQuestions = vi.fn();
const saveProjectAnswers = vi.fn();
const setupSteps = vi.fn();
const startRun = vi.fn();
const streamRun = vi.fn();
const runsForDir = vi.fn();
const frameworkCatalogue = vi.fn();

vi.mock('$stores/wizard', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/wizard')>();
  return {
    ...actual,
    projectQuestions: (...a: unknown[]) => projectQuestions(...a),
    saveProjectAnswers: (...a: unknown[]) => saveProjectAnswers(...a),
    setupSteps: (...a: unknown[]) => setupSteps(...a),
    startRun: (...a: unknown[]) => startRun(...a),
    streamRun: (...a: unknown[]) => streamRun(...a),
    runsForDir: (...a: unknown[]) => runsForDir(...a),
    frameworkCatalogue: (...a: unknown[]) => frameworkCatalogue(...a)
  };
});

const browseDir = vi.fn();
vi.mock('$stores/browse', () => ({ browseDir: (...a: unknown[]) => browseDir(...a) }));

vi.mock('$stores/sites', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/sites')>();
  return { ...actual, loadSites: vi.fn() };
});

const goToTab = vi.fn();
vi.mock('$stores/route', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/route')>();
  return { ...actual, goToTab: (...a: unknown[]) => goToTab(...a) };
});

const questions = {
  dir: '/home/u/acme',
  kind: 'php',
  kind_choice: false,
  php_version: '8.3',
  php_installed: ['8.3', '8.2'],
  node_managed: false,
  https_available: true,
  secured: false,
  database_options: [
    { value: 'sqlite', label: 'SQLite (no service)' },
    { value: 'mysql', label: 'MySQL' }
  ],
  database: 'sqlite',
  service_options: ['redis', 'mailpit'],
  services: [],
  frankenphp_offered: false,
  frankenphp: false,
  frankenphp_worker: false,
  worker_options: ['queue'],
  workers: []
};

function finishedRun(id: string) {
  return { id, kind: 'link', dir: '/home/u/acme', status: 'done' as const, started: 0 };
}

describe('SiteWizardModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    sites.set([]);
    browseDir.mockImplementation(async (dir: string) => ({
      current: dir || '/home/u',
      dirs: [{ name: 'acme', path: (dir || '/home/u') + '/acme' }]
    }));
    projectQuestions.mockResolvedValue(questions);
    saveProjectAnswers.mockResolvedValue(undefined);
    setupSteps.mockResolvedValue([
      { label: 'composer install', enabled: true, optional: false },
      { label: 'npm run build', enabled: false, optional: false }
    ]);
    startRun.mockImplementation(async (req: { kind: string }) => finishedRun('run-' + req.kind));
    streamRun.mockImplementation(async (_id: string, onEvent: (e: unknown) => void) => {
      onEvent({ line: 'working' });
      onEvent({ done: true, ok: true });
    });
    runsForDir.mockResolvedValue([]);
    frameworkCatalogue.mockResolvedValue([
      { name: 'laravel', label: 'Laravel', versions: ['12', '11'], latest: '12' }
    ]);
  });

  // The + used to open a directory browser and nothing else. It now asks what
  // the user is doing first.
  it('opens on the choice between linking and creating', async () => {
    render(SiteWizardModal);
    expect(await screen.findByText('Link an existing project')).toBeTruthy();
    expect(screen.getByText('Create a new project')).toBeTruthy();
  });

  it('takes the link path through the questions and the setup steps', async () => {
    render(SiteWizardModal);

    await fireEvent.click(await screen.findByText('Link an existing project'));
    await fireEvent.click(await screen.findByText('acme'));
    await fireEvent.click(screen.getByText('Link This Directory'));

    // The questions the terminal wizard asks, for the directory that was picked.
    await waitFor(() => expect(projectQuestions).toHaveBeenCalledWith('/home/u/acme'));
    expect(await screen.findByText('PHP version')).toBeTruthy();
    expect(screen.getByText('Database')).toBeTruthy();

    await fireEvent.click(screen.getByText('Continue'));

    // The answers are saved, then the project is linked and its env set up.
    await waitFor(() => expect(saveProjectAnswers).toHaveBeenCalled());
    await waitFor(() => {
      const kinds = startRun.mock.calls.map((c) => (c[0] as { kind: string }).kind);
      expect(kinds).toContain('link');
      expect(kinds).toContain('env');
    });

    // And it ends on the setup steps, pre-ticked the way the CLI selector is.
    expect(await screen.findByText('composer install')).toBeTruthy();
    const boxes = document.querySelectorAll<HTMLInputElement>('input[type="checkbox"]');
    expect(boxes[0].checked).toBe(true);
    expect(boxes[1].checked).toBe(false);
  });

  it('opens the next link in the folder it was told to remember', async () => {
    const first = render(SiteWizardModal);
    await fireEvent.click(await screen.findByText('Link an existing project'));
    await fireEvent.click(await screen.findByText('acme'));
    await fireEvent.click(screen.getByText('Start here next time'));
    await fireEvent.click(screen.getByText('Link This Directory'));
    await screen.findByText('PHP version');
    first.unmount();
    localStorage.removeItem('lerd.siteWizard');

    browseDir.mockClear();
    render(SiteWizardModal);
    await fireEvent.click(await screen.findByText('Link an existing project'));
    await waitFor(() => expect(browseDir).toHaveBeenCalledWith('/home/u'));
    expect(screen.getByLabelText('Start here next time')).toHaveProperty('checked', true);
  });

  it('falls back to home when the remembered folder is gone', async () => {
    localStorage.setItem('lerd.siteWizard.sitesFolder', '/home/u/gone');
    browseDir.mockImplementation(async (dir: string) =>
      dir === '/home/u/gone'
        ? { error: 'no such directory' }
        : { current: dir || '/home/u', dirs: [{ name: 'acme', path: (dir || '/home/u') + '/acme' }] }
    );
    render(SiteWizardModal);
    await fireEvent.click(await screen.findByText('Link an existing project'));
    expect(await screen.findByText('acme')).toBeTruthy();
    expect(localStorage.getItem('lerd.siteWizard.sitesFolder')).toBeNull();
  });

  it('starts at home when nothing is remembered', async () => {
    render(SiteWizardModal);
    await fireEvent.click(await screen.findByText('Link an existing project'));
    await fireEvent.click(await screen.findByText('acme'));
    await fireEvent.click(screen.getByText('Link This Directory'));
    await screen.findByText('PHP version');
    expect(localStorage.getItem('lerd.siteWizard.sitesFolder')).toBeNull();
  });

  it('runs the selected setup steps one at a time', async () => {
    render(SiteWizardModal);
    await fireEvent.click(await screen.findByText('Link an existing project'));
    await fireEvent.click(await screen.findByText('acme'));
    await fireEvent.click(screen.getByText('Link This Directory'));
    await screen.findByText('PHP version');
    await fireEvent.click(screen.getByText('Continue'));
    await screen.findByText('composer install');

    await fireEvent.click(screen.getByText('Run setup'));

    await waitFor(() => {
      const setupCalls = startRun.mock.calls
        .map((c) => c[0] as { kind: string; steps?: string[] })
        .filter((c) => c.kind === 'setup');
      expect(setupCalls).toHaveLength(1);
      expect(setupCalls[0].steps).toEqual(['composer install']);
    });
  });

  it('scaffolds a new project and carries it into the questions', async () => {
    render(SiteWizardModal);

    await fireEvent.click(await screen.findByText('Create a new project'));
    await fireEvent.click(await screen.findByText('acme'));
    await fireEvent.click(screen.getByText('Use this folder'));

    const nameField = await screen.findByPlaceholderText('my-app');
    await fireEvent.input(nameField, { target: { value: 'shop' } });
    await fireEvent.click(screen.getByText('Create project'));

    await waitFor(() => {
      expect(startRun).toHaveBeenCalledWith(
        expect.objectContaining({ kind: 'scaffold', dir: '/home/u/acme', name: 'shop', framework: 'laravel' })
      );
    });
    // The scaffolded directory is what gets configured, not the parent.
    await waitFor(() => expect(projectQuestions).toHaveBeenCalledWith('/home/u/acme/shop'));
  });

  // Closing the modal parks the wizard; reopening reattaches to the run that
  // kept going on the host rather than starting it over.
  it('reattaches to a run that is still going', async () => {
    localStorage.setItem(
      'lerd.siteWizard',
      JSON.stringify({
        step: 'questions',
        dir: '/home/u/acme/shop',
        parent: '/home/u/acme',
        name: 'shop',
        runId: 'run-scaffold',
        runKind: 'scaffold'
      })
    );
    runsForDir.mockResolvedValue([
      { id: 'run-scaffold', kind: 'scaffold', dir: '/home/u/acme', status: 'running', started: 0 }
    ]);

    render(SiteWizardModal);

    await waitFor(() => expect(streamRun).toHaveBeenCalledWith('run-scaffold', expect.any(Function)));
    expect(startRun).not.toHaveBeenCalled();
  });

  // Sending a run to the background closes the modal and leaves the work going.
  // Reopening carries on from where it got to rather than starting over: here
  // the scaffold finished while the modal was closed, so the wizard picks up at
  // the questions for the directory it created.
  it('carries on from a run that finished while the modal was closed', async () => {
    localStorage.setItem(
      'lerd.siteWizard',
      JSON.stringify({
        step: 'create',
        dir: '',
        parent: '/home/u/acme',
        name: 'shop',
        runId: 'run-scaffold',
        runKind: 'scaffold'
      })
    );
    runsForDir.mockResolvedValue([
      { id: 'run-scaffold', kind: 'scaffold', dir: '/home/u/acme', status: 'done', started: 0 }
    ]);

    render(SiteWizardModal);

    await waitFor(() => expect(projectQuestions).toHaveBeenCalledWith('/home/u/acme/shop'));
    expect(startRun).not.toHaveBeenCalled();
  });

  // A parked setup comes back to the steps it had left, not to an empty list.
  it('drains the rest of the setup queue on resume', async () => {
    localStorage.setItem(
      'lerd.siteWizard',
      JSON.stringify({
        step: 'setup',
        dir: '/home/u/acme',
        runId: 'run-setup',
        runKind: 'setup',
        queue: ['composer install', 'npm run build']
      })
    );
    runsForDir.mockResolvedValue([
      { id: 'run-setup', kind: 'setup', dir: '/home/u/acme', status: 'done', started: 0 }
    ]);

    render(SiteWizardModal);

    await waitFor(() => {
      const setupCalls = startRun.mock.calls
        .map((c) => c[0] as { kind: string; steps?: string[] })
        .filter((c) => c.kind === 'setup');
      expect(setupCalls).toHaveLength(1);
      expect(setupCalls[0].steps).toEqual(['npm run build']);
    });
  });

  // The plan is what says which steps are optional. A resumed queue that never
  // loaded it would treat every remaining step as required, so one optional
  // failure would stop the rest.
  it('keeps optional steps optional when the queue resumes', async () => {
    localStorage.setItem(
      'lerd.siteWizard',
      JSON.stringify({
        step: 'setup',
        dir: '/home/u/acme',
        runId: 'run-setup-head',
        runKind: 'setup',
        queue: ['composer install', 'npm audit', 'npm run build']
      })
    );
    runsForDir.mockResolvedValue([
      { id: 'run-setup-head', kind: 'setup', dir: '/home/u/acme', status: 'done', started: 0 }
    ]);
    setupSteps.mockResolvedValue([
      { label: 'composer install', enabled: true, optional: false },
      { label: 'npm audit', enabled: true, optional: true },
      { label: 'npm run build', enabled: true, optional: false }
    ]);
    startRun.mockImplementation(async (req: { kind: string; steps?: string[] }) => ({
      id: 'run-' + (req.steps?.[0] ?? req.kind),
      kind: req.kind,
      dir: '/home/u/acme',
      status: 'done' as const,
      started: 0
    }));
    streamRun.mockImplementation(async (id: string, onEvent: (e: unknown) => void) => {
      onEvent({ done: true, ok: id !== 'run-npm audit' });
    });

    render(SiteWizardModal);

    await waitFor(() => {
      const setupCalls = startRun.mock.calls
        .map((c) => c[0] as { kind: string; steps?: string[] })
        .filter((c) => c.kind === 'setup')
        .map((c) => c.steps?.[0]);
      // The optional audit failed and the build after it still ran.
      expect(setupCalls).toEqual(['npm audit', 'npm run build']);
    });
  });

  // A finished run is only kept for so long. A scaffold parked past that is
  // still a scaffolded project on disk, and reopening the wizard carries it
  // into the questions instead of dead-ending on the create form.
  it('continues a scaffold whose run has aged out of the registry', async () => {
    localStorage.setItem(
      'lerd.siteWizard',
      JSON.stringify({
        step: 'create',
        dir: '/home/u/acme',
        parent: '/home/u',
        name: 'acme',
        runId: 'run-scaffold',
        runKind: 'scaffold'
      })
    );
    runsForDir.mockResolvedValue([]);

    render(SiteWizardModal);

    await waitFor(() => {
      expect(projectQuestions).toHaveBeenCalledWith('/home/u/acme');
    });
  });

  // Coming back to a run that is still going has to show the run, not a
  // spinner: watching the output is the whole point of reopening it.
  it('shows the live run rather than a loader when it reattaches', async () => {
    localStorage.setItem(
      'lerd.siteWizard',
      JSON.stringify({ step: 'setup', dir: '/home/u/acme', runId: 'run-setup', runKind: 'setup' })
    );
    runsForDir.mockResolvedValue([
      {
        id: 'run-setup',
        kind: 'setup',
        dir: '/home/u/acme',
        label: 'npm install/ci',
        status: 'running',
        started: 0
      }
    ]);
    // A run in flight: it prints, and does not end while the modal is open.
    streamRun.mockImplementation(
      (_id: string, onEvent: (e: unknown) => void) =>
        new Promise(() => {
          onEvent({ line: 'added 120 packages' });
        })
    );

    render(SiteWizardModal);

    expect(await screen.findByText('added 120 packages')).toBeTruthy();
    expect(screen.queryByText('Loading...')).toBeNull();
    // And the footer offers to send it back to the background, not a spinner.
    expect(screen.getByText('Run in the background')).toBeTruthy();
  });

  // The site is what the wizard ends on, and a resumed flow never watched the
  // link that registered it, so the domain has to come from the site list.
  it('opens the site at the end of a resumed flow', async () => {
    sites.set([{ domain: 'acme.test', path: '/home/u/acme' } as never]);
    localStorage.setItem(
      'lerd.siteWizard',
      JSON.stringify({ step: 'setup', dir: '/home/u/acme' })
    );

    render(SiteWizardModal);
    await screen.findByText('composer install');
    await fireEvent.click(screen.getByText('Skip setup'));

    await waitFor(() => expect(goToTab).toHaveBeenCalledWith('sites', 'acme.test'));
  });

  // No step is ever required, so a list with nothing ticked just finishes.
  it('offers only Open the site while no setup step is ticked', async () => {
    sites.set([{ domain: 'acme.test', path: '/home/u/acme' } as never]);
    setupSteps.mockResolvedValue([{ label: 'lerd mcp:inject', enabled: false, optional: false }]);
    localStorage.setItem('lerd.siteWizard', JSON.stringify({ step: 'setup', dir: '/home/u/acme' }));

    render(SiteWizardModal);
    await screen.findByText('lerd mcp:inject');

    expect(screen.queryByText('Run setup')).toBeNull();
    expect(screen.queryByText('Skip setup')).toBeNull();
    await fireEvent.click(screen.getByText('lerd mcp:inject'));
    expect(await screen.findByText('Run setup')).toBeTruthy();
    expect(screen.getByText('Skip setup')).toBeTruthy();
    await fireEvent.click(screen.getByText('lerd mcp:inject'));

    await fireEvent.click(await screen.findByText('Open the site'));
    await waitFor(() => expect(goToTab).toHaveBeenCalledWith('sites', 'acme.test'));
  });

  // A directory that already is a site has nothing left to ask. Walking the
  // questions again would link it twice; the wizard hands over to the site.
  it('opens a directory that is already linked instead of relinking it', async () => {
    sites.set([{ domain: 'acme.test', path: '/home/u/acme' } as never]);

    render(SiteWizardModal);
    await fireEvent.click(await screen.findByText('Link an existing project'));
    await fireEvent.click(await screen.findByText('acme'));

    await fireEvent.click(await screen.findByText('Open acme.test'));

    await waitFor(() => expect(goToTab).toHaveBeenCalledWith('sites', 'acme.test'));
    expect(projectQuestions).not.toHaveBeenCalled();
    expect(startRun).not.toHaveBeenCalled();
  });

  it('reports a failed run instead of moving on', async () => {
    streamRun.mockImplementation(async (_id: string, onEvent: (e: unknown) => void) => {
      onEvent({ done: true, ok: false, error: 'composer could not resolve dependencies' });
    });

    render(SiteWizardModal);
    await fireEvent.click(await screen.findByText('Link an existing project'));
    await fireEvent.click(await screen.findByText('acme'));
    await fireEvent.click(screen.getByText('Link This Directory'));
    await screen.findByText('PHP version');
    await fireEvent.click(screen.getByText('Continue'));

    expect(await screen.findByText('composer could not resolve dependencies')).toBeTruthy();
    expect(setupSteps).not.toHaveBeenCalled();
  });
});
