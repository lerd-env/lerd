<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { fly, slide } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import Icon from '$components/Icon.svelte';
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import BuildLog from '$components/BuildLog.svelte';
  import WizardStart from './wizard/WizardStart.svelte';
  import WizardBrowse from './wizard/WizardBrowse.svelte';
  import { finishProjectName } from '$lib/projectName';
  import WizardCreate from './wizard/WizardCreate.svelte';
  import WizardQuestions from './wizard/WizardQuestions.svelte';
  import WizardSetup from './wizard/WizardSetup.svelte';
  import WizardBusy from './wizard/WizardBusy.svelte';
  import { closeModal } from '$stores/modals';
  import { sites, loadSites } from '$stores/sites';
  import { goToTab } from '$stores/route';
  import {
    projectQuestions,
    saveProjectAnswers,
    setupSteps,
    startRun,
    streamRun,
    runsForDir,
    refreshActiveRun,
    watchActiveRun,
    type ProjectAnswers,
    type ProjectQuestions,
    type RunRequest,
    type SetupStep
  } from '$stores/wizard';
  import { clearWizardState, loadWizardState, saveWizardState } from '$lib/wizardState';
  import { m } from '../paraglide/messages.js';

  type Step = 'start' | 'browse' | 'parent' | 'create' | 'questions' | 'setup' | 'done';

  let step = $state<Step>('start');
  let dir = $state('');
  let parent = $state('');
  let name = $state('');
  let framework = $state('');
  let frameworkVersion = $state('');

  let questions = $state<ProjectQuestions | null>(null);
  let answers = $state<ProjectAnswers | null>(null);
  let steps = $state<SetupStep[]>([]);
  let selectedSteps = $state<string[]>([]);
  let queue = $state<string[]>([]);
  let finishedSteps = $state<Array<{ label: string; ok: boolean; error?: string }>>([]);
  let currentStep = $state('');

  let running = $state(false);
  let runTitle = $state('');
  // The run this wizard is attached to. Held here rather than only written at
  // the start, so every save keeps pointing at it: closing the modal mid-run
  // used to drop the pointer, and reopening then started the work again.
  let runId = $state('');
  let runKind = $state('');
  // Closing the modal ends this component's part in the flow. The run on the
  // host carries on, but the queue behind it stops here: a destroyed modal that
  // kept starting steps would race the one that reopens to resume them.
  let alive = true;
  let logs = $state<string[]>([]);
  let busy = $state(false);
  let resuming = $state(true);
  let error = $state('');
  let warning = $state('');
  const domain = $derived($sites.find((s) => s.path === dir)?.domain ?? '');

  // Directories that are already sites, so the browser can mark them and the
  // wizard can offer the site instead of linking it a second time.
  const linkedPaths = $derived(
    Object.fromEntries($sites.filter((s) => s.path).map((s) => [s.path as string, s.domain]))
  );

  // The project the wizard is on, for the steps that are about one directory.
  const project = $derived(dir.split('/').filter(Boolean).pop() ?? dir);

  // The header says where in the flow you are rather than how you got here: by
  // the setup step the project exists, so "Create a project" is no longer true.
  // While something runs, the header is what is running.
  const title = $derived.by(() => {
    if (running) return runTitle;
    switch (step) {
      case 'browse':
        return m.siteWizard_titleBrowse();
      case 'parent':
        return m.siteWizard_titleParent();
      case 'create':
        return m.siteWizard_createTitle();
      case 'questions':
        return m.siteWizard_titleConfigure({ project });
      case 'setup':
        return m.siteWizard_titleSetup({ project });
      case 'done':
        return m.siteWizard_readyTitle();
      default:
        return m.siteWizard_title();
    }
  });

  function persist() {
    saveWizardState({
      step,
      dir,
      parent,
      name,
      framework,
      frameworkVersion,
      queue,
      runId: runId || undefined,
      runKind: runKind || undefined
    });
  }

  function fail(message: string) {
    error = message;
    running = false;
    busy = false;
  }

  function joinPath(base: string, child: string): string {
    return base.endsWith('/') ? base + child : base + '/' + child;
  }

  // runAndWait starts a run on the host and follows it to its end. The run is
  // registered in lerd-ui before this returns, so closing the modal or
  // reloading the page leaves the work going and only drops this view of it.
  async function runAndWait(req: RunRequest, heading: string): Promise<boolean> {
    running = true;
    runTitle = heading;
    logs = [];
    error = '';
    try {
      const started = await startRun(req);
      runId = started.id;
      runKind = req.kind;
      persist();
      // The button that reopens this modal spins on the same signal, so it
      // starts spinning the moment the work does, not on the next poll.
      watchActiveRun();
      const ok = await follow(started.id);
      // A run that succeeded has been acted on by the caller that follows this
      // line, so it is no longer what a resume should reattach to. A failed one
      // stays, so reopening shows why it failed rather than a blank step.
      if (ok) {
        runId = '';
        runKind = '';
      }
      return ok;
    } catch (e) {
      fail(e instanceof Error ? e.message : m.common_failed());
      return false;
    }
  }

  // follow attaches to a run, replaying what it already printed. Reattaching is
  // what a reloaded page does with a scaffold that is still going.
  async function follow(id: string): Promise<boolean> {
    let ok = false;
    let failure = '';
    await streamRun(id, (ev) => {
      if (ev.done) {
        ok = Boolean(ev.ok);
        failure = ev.error ?? '';
        return;
      }
      if (ev.line !== undefined) logs = [...logs, ev.line];
    });
    running = false;
    void refreshActiveRun();
    if (!ok) error = failure || m.common_failed();
    return ok;
  }

  async function loadQuestions() {
    busy = true;
    try {
      const q = await projectQuestions(dir);
      questions = q;
      answers = {
        kind: q.kind,
        php_version: q.php_version,
        node_version: q.node_version,
        secured: q.secured,
        database: q.database,
        services: q.services ?? [],
        frankenphp: q.frankenphp,
        frankenphp_worker: q.frankenphp_worker,
        workers: q.workers ?? [],
        proxy_command: q.proxy_command,
        proxy_port: q.proxy_port,
        container_port: q.container_port,
        containerfile: q.containerfile
      };
      step = 'questions';
      persist();
    } catch (e) {
      fail(e instanceof Error ? e.message : m.common_failed());
    } finally {
      busy = false;
    }
  }

  async function loadSetupSteps() {
    try {
      steps = await setupSteps(dir);
      selectedSteps = steps.filter((s) => s.enabled).map((s) => s.label);
    } catch (e) {
      // A project whose steps cannot be planned still ends on a linked site.
      steps = [];
      selectedSteps = [];
      warning = e instanceof Error ? e.message : m.common_failed();
    }
    step = 'setup';
    persist();
  }

  async function scaffold() {
    if (!finishProjectName(name)) return;
    // The target is known before the run starts, and persisting it now is what
    // lets a resume carry on into the questions even after the finished run has
    // aged out of the registry.
    dir = joinPath(parent, finishProjectName(name));
    const ok = await runAndWait(
      { kind: 'scaffold', dir: parent, name: finishProjectName(name), framework, framework_version: frameworkVersion },
      m.siteWizard_scaffolding()
    );
    if (!ok) return;
    await afterRun('scaffold');
  }

  // configure saves the answers as .lerd.yaml and takes the project through the
  // same link and environment setup the terminal runs, ending on the step list.
  async function configure() {
    if (!answers) return;
    busy = true;
    try {
      await saveProjectAnswers(dir, answers);
    } catch (e) {
      fail(e instanceof Error ? e.message : m.common_failed());
      return;
    } finally {
      busy = false;
    }

    if (!(await runAndWait({ kind: 'link', dir }, m.siteWizard_linking()))) return;
    if (!alive) return;

    // The site is linked either way, so an environment that did not finish is a
    // warning to read rather than an error that stops the wizard.
    await afterRun('link');
  }

  // linkedSite refreshes the site list the link just added to, so the wizard can
  // end on the new site, then moves on to its steps.
  async function linkedSite() {
    await loadSites();
    await loadSetupSteps();
  }

  // afterRun is what a finished run owes the flow. The wizard calls it when the
  // run finishes in front of you, and resume calls it for a run that finished
  // while the modal was closed, so both paths continue the same way.
  async function afterRun(kind: string) {
    switch (kind) {
      case 'scaffold':
        dir = joinPath(parent, name);
        await loadQuestions();
        return;
      case 'link': {
        const envOk = await runAndWait({ kind: 'env', dir }, m.siteWizard_configuringEnv());
        if (!alive) return;
        if (!envOk) {
          warning = error || m.link_envWarning();
          error = '';
        }
        await linkedSite();
        return;
      }
      case 'env':
        await linkedSite();
        return;
      case 'setup':
        // The step that just finished is the head of the saved queue.
        if (queue.length > 0) {
          const label = queue[0];
          finishedSteps = [...finishedSteps, { label, ok: true }];
          queue = queue.slice(1);
        }
        await drainQueue();
    }
  }

  async function runSetup() {
    queue = steps.filter((s) => selectedSteps.includes(s.label)).map((s) => s.label);
    finishedSteps = [];
    await drainQueue();
  }

  async function drainQueue() {
    while (alive && queue.length > 0) {
      const label = queue[0];
      currentStep = label;
      const optional = steps.find((s) => s.label === label)?.optional ?? false;
      const ok = await runAndWait({ kind: 'setup', dir, steps: [label] }, label);
      if (!alive) return;
      finishedSteps = [...finishedSteps, { label, ok, error: ok ? '' : error }];
      queue = queue.slice(1);
      if (!ok && !optional) {
        // Stop where it broke: the rest of the steps usually depend on it, and
        // the site is linked already, so the wizard can still end on it.
        currentStep = '';
        return;
      }
      error = '';
    }
    currentStep = '';
    step = 'done';
    persist();
  }

  async function finish() {
    // A flow that was resumed may never have refreshed the site list in this
    // tab, and the wizard is not much use if it cannot end on the site.
    if (!domain) await loadSites();
    const target = domain;
    clearWizardState();
    closeModal();
    if (target) goToTab('sites', target);
  }

  function cancel() {
    clearWizardState();
    closeModal();
  }

  // Closing the modal parks the wizard rather than cancelling it: whatever is
  // running on the host keeps going, and reopening picks it back up.
  function park() {
    persist();
    closeModal();
  }

  // A directory that is already served has nothing left to ask: the wizard
  // hands over to the site rather than walking the questions again.
  function openExisting() {
    const target = domain;
    clearWizardState();
    closeModal();
    if (target) goToTab('sites', target);
  }

  function chooseMode(next: 'link' | 'create') {
    step = next === 'create' ? 'parent' : 'browse';
    persist();
  }

  // resume picks the wizard back up where it was left: reattached to a run that
  // is still going, or carrying on from one that finished while the modal was
  // closed. Sending a run to the background is a pause, not a cancellation.
  async function resume() {
    try {
      const saved = loadWizardState();
      if (!saved) return;
      step = (saved.step as Step) ?? 'start';
      dir = saved.dir ?? '';
      parent = saved.parent ?? '';
      name = saved.name ?? '';
      framework = saved.framework ?? '';
      frameworkVersion = saved.frameworkVersion ?? '';
      queue = saved.queue ?? [];
      runId = saved.runId ?? '';
      runKind = saved.runKind ?? '';

      // A run this wizard started, still in the registry: following it replays
      // what it printed, and returns straight away when it has already ended.
      const known = saved.runId
        ? (await runsForDir(saved.runKind === 'scaffold' ? parent : dir)).find(
            (r) => r.id === saved.runId
          )
        : undefined;

      if (known) {
        running = true;
        runTitle = known.label || m.siteWizard_scaffolding();
        logs = [];
        // The queue that continues after a setup run reads each step's optional
        // flag off the plan, which a reopened modal has not loaded yet; without
        // it every remaining step counts as required and one optional failure
        // stops the rest.
        if (runKind === 'setup' && dir) {
          try {
            steps = await setupSteps(dir);
          } catch {
            steps = [];
          }
        }
        // The run panel takes the body from here: following a run that is still
        // going does not return until it ends, and holding the loader up for
        // that long is what hid the output the user came back to watch.
        resuming = false;
        if (await follow(known.id)) {
          runId = '';
          runKind = '';
          await afterRun(saved.runKind ?? '');
        }
        return;
      }

      // Nothing left to reattach to: rebuild the step from the project on disk.
      if (!dir) return;
      resuming = false;
      // A scaffold whose run has aged out of the registry left its project on
      // disk; the questions are what comes after it, same as afterRun.
      if (step === 'create') await loadQuestions();
      if (step === 'questions') await loadQuestions();
      if (step === 'setup') await loadSetupSteps();
    } finally {
      resuming = false;
    }
  }

  onMount(() => {
    void resume();
  });

  onDestroy(() => {
    alive = false;
  });
</script>

<Modal open {title} onclose={running ? park : cancel} size={step === 'questions' ? 'xl' : 'lg'}>
  {#if step !== 'start' && dir}
    <div class="px-5 py-3 border-b border-gray-100 dark:border-lerd-border" transition:slide={{ duration: 160 }}>
      <div class="text-xs text-gray-400 mb-1">{m.link_directory()}</div>
      <div class="text-sm font-mono text-gray-700 dark:text-gray-300 truncate">{dir}</div>
    </div>
  {/if}

  {#key running ? 'run:' + runTitle : step}
    <div in:fly={{ y: 6, duration: 200, easing: cubicOut }}>
    {#if resuming || (busy && step !== 'questions')}
      <WizardBusy label={m.common_loading()} />
    {:else if running}
      <div class="px-5 py-3 space-y-2">
        <div class="flex items-center gap-2 text-xs font-medium text-gray-500 dark:text-gray-400">
          <Icon name="spinner" class="w-3.5 h-3.5 animate-spin" />
          {runTitle}
        </div>
        <BuildLog {logs} />
      </div>
    {:else if step === 'start'}
      <WizardStart onchoose={chooseMode} />
    {:else if step === 'browse' || step === 'parent'}
      <div class="px-5 py-3 border-b border-gray-100 dark:border-lerd-border">
        <div class="text-xs text-gray-400 mb-1">
          {step === 'parent' ? m.siteWizard_chooseParent() : m.link_directory()}
        </div>
        <div class="text-sm font-mono text-gray-700 dark:text-gray-300 truncate">
          {(step === 'parent' ? parent : dir) || '...'}
        </div>
      </div>
      <WizardBrowse
        dir={step === 'parent' ? parent : dir}
        onnavigate={(d) => (step === 'parent' ? (parent = d) : (dir = d))}
        onerror={(msg) => (error = msg)}
      />
    {:else if step === 'create'}
      <WizardCreate
        {parent}
        {name}
        {framework}
        {frameworkVersion}
        onchange={(v) => {
          name = v.name;
          framework = v.framework;
          frameworkVersion = v.frameworkVersion;
        }}
        onerror={(msg) => (error = msg)}
      />
    {:else if step === 'questions' && questions && answers}
      <WizardQuestions {questions} {answers} onchange={(a) => (answers = a)} />
    {:else if step === 'setup'}
      <WizardSetup
        {steps}
        selected={selectedSteps}
        finished={finishedSteps}
        current={currentStep}
        onchange={(s) => (selectedSteps = s)}
      />
    {:else if step === 'done'}
      <div class="px-5 py-4">
        <p class="text-sm text-gray-700 dark:text-gray-300">{m.siteWizard_ready()}</p>
      </div>
    {/if}
    </div>
  {/key}

  {#if error}
    <div class="px-5 py-2" transition:slide={{ duration: 160 }}>
      <p class="text-xs text-red-500 break-words">{error}</p>
    </div>
  {/if}

  {#if warning}
    <div class="px-5 py-2" transition:slide={{ duration: 160 }}>
      <p class="text-xs text-amber-600 dark:text-amber-500 break-words">{warning}</p>
    </div>
  {/if}

  {#snippet footer()}
    {#if resuming}
      <DetailButton onclick={cancel}>{m.common_cancel()}</DetailButton>
    {:else if running}
      <DetailButton onclick={park}>{m.siteWizard_hide()}</DetailButton>
    {:else if step === 'start'}
      <DetailButton onclick={cancel}>{m.common_cancel()}</DetailButton>
    {:else if step === 'browse'}
      <DetailButton onclick={cancel}>{m.common_cancel()}</DetailButton>
      {#if domain}
        <DetailButton tone="primary" onclick={openExisting}>
          {m.siteWizard_openExisting({ domain })}
        </DetailButton>
      {:else}
        <DetailButton tone="primary" onclick={loadQuestions} disabled={!dir || busy} loading={busy}>
          {m.link_linkThisDir()}
        </DetailButton>
      {/if}
    {:else if step === 'parent'}
      <DetailButton onclick={cancel}>{m.common_cancel()}</DetailButton>
      <DetailButton
        tone="primary"
        onclick={() => {
          step = 'create';
          persist();
        }}
        disabled={!parent}
      >
        {m.siteWizard_useThisFolder()}
      </DetailButton>
    {:else if step === 'create'}
      <DetailButton
        onclick={() => {
          step = 'parent';
          persist();
        }}
      >
        {m.siteWizard_back()}
      </DetailButton>
      <DetailButton tone="primary" onclick={scaffold} disabled={!finishProjectName(name) || !framework}>
        {m.siteWizard_create()}
      </DetailButton>
    {:else if step === 'questions'}
      <DetailButton onclick={cancel}>{m.common_cancel()}</DetailButton>
      <DetailButton tone="primary" onclick={configure} disabled={busy} loading={busy}>
        {m.siteWizard_continue()}
      </DetailButton>
    {:else if step === 'setup'}
      {#if finishedSteps.length > 0 || steps.length === 0}
        <DetailButton tone="primary" onclick={finish}>{m.siteWizard_finish()}</DetailButton>
      {:else}
        <DetailButton onclick={finish}>{m.siteWizard_skipSetup()}</DetailButton>
        <DetailButton tone="primary" onclick={runSetup} disabled={selectedSteps.length === 0}>
          {m.siteWizard_runSetup()}
        </DetailButton>
      {/if}
    {:else if step === 'done'}
      <DetailButton tone="primary" onclick={finish}>{m.siteWizard_finish()}</DetailButton>
    {/if}
  {/snippet}
</Modal>
