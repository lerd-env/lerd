<script lang="ts">
  import { slide } from 'svelte/transition';
  import Dropdown from '$components/Dropdown.svelte';
  import WizardField from './WizardField.svelte';
  import WizardCheckList from './WizardCheckList.svelte';
  import type { ProjectAnswers, ProjectQuestions } from '$stores/wizard';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    questions: ProjectQuestions;
    answers: ProjectAnswers;
    onchange: (answers: ProjectAnswers) => void;
  }
  let { questions, answers, onchange }: Props = $props();

  const phpOptions = $derived(() => {
    const installed = questions.php_installed ?? [];
    const current = answers.php_version ?? '';
    // A project pinned to a version this machine has not installed still shows
    // its own answer: link installs it, and hiding it would silently move the
    // project to another version.
    const values = current && !installed.includes(current) ? [current, ...installed] : installed;
    return values.map((v) => ({ value: v, label: v }));
  });

  // Node is picked from what lerd has installed, plus whatever the project
  // already pins, and the empty answer is the project resolving its own version.
  const nodeOptions = $derived(() => {
    const installed = questions.node_installed ?? [];
    const current = answers.node_version ?? '';
    const values = current && !installed.includes(current) ? [current, ...installed] : installed;
    return [
      { value: '', label: m.siteWizard_nodeUnpinnedOption() },
      ...values.map((v) => ({ value: v, label: v }))
    ];
  });

  const serviceItems = $derived(
    (questions.service_options ?? []).map((s) => ({ value: s, label: s }))
  );
  const workerItems = $derived((questions.worker_options ?? []).map((w) => ({ value: w, label: w })));

  function set(next: Partial<ProjectAnswers>) {
    onchange({ ...answers, ...next });
  }

  function port(value: string): number {
    const parsed = parseInt(value, 10);
    return Number.isNaN(parsed) ? 0 : parsed;
  }
</script>

<div class="px-5 py-4 space-y-4 max-h-[65vh] overflow-y-auto">
  {#if questions.kind_choice && (questions.kind_options ?? []).length > 0}
    <WizardField label={questions.kind_title ?? ''}>
      <Dropdown
        value={answers.kind}
        width="full"
        options={(questions.kind_options ?? []).map((o) => ({ value: o.value, label: o.label }))}
        onchange={(v) => set({ kind: v })}
      />
    </WizardField>
  {/if}

  {#if answers.kind === 'php'}
    <div class="space-y-4" transition:slide={{ duration: 160 }}>
    <WizardField label={m.siteWizard_phpVersion()}>
      <Dropdown
        value={answers.php_version ?? ''}
        width="full"
        options={phpOptions()}
        onchange={(v) => set({ php_version: v })}
      />
    </WizardField>

    {#if questions.node_managed}
      <WizardField
        label={m.siteWizard_nodeVersion()}
        hint={questions.node_version_of
          ? m.siteWizard_nodeFollow({ source: questions.node_version_of })
          : m.siteWizard_nodeUnpinned()}
      >
        <Dropdown
          value={answers.node_version ?? ''}
          width="full"
          options={nodeOptions()}
          onchange={(v) => set({ node_version: v })}
        />
      </WizardField>
    {/if}
    </div>
  {/if}

  {#if answers.kind === 'proxy'}
    <div class="space-y-4" transition:slide={{ duration: 160 }}>
    <WizardField
      label={m.siteWizard_devCommand()}
      hint={questions.proxy_command_hint
        ? m.siteWizard_devCommandDetected({ scripts: questions.proxy_command_hint })
        : m.siteWizard_devCommandHint()}
    >
      <input
        type="text"
        value={answers.proxy_command ?? ''}
        oninput={(e) => set({ proxy_command: (e.currentTarget as HTMLInputElement).value })}
        class="w-full text-sm bg-white dark:bg-lerd-bg border border-gray-200 dark:border-lerd-border rounded-sm px-2 py-1.5 text-gray-700 dark:text-gray-300 focus:outline-hidden focus:border-lerd-red/50"
      />
    </WizardField>

    <WizardField label={m.siteWizard_port()}>
      <input
        type="number"
        value={answers.proxy_port ?? 0}
        oninput={(e) => set({ proxy_port: port((e.currentTarget as HTMLInputElement).value) })}
        class="w-full text-sm bg-white dark:bg-lerd-bg border border-gray-200 dark:border-lerd-border rounded-sm px-2 py-1.5 text-gray-700 dark:text-gray-300 focus:outline-hidden focus:border-lerd-red/50"
      />
    </WizardField>

    {#if questions.proxy_vite_pitfall}
      <p class="text-xs text-amber-600 dark:text-amber-500">{m.siteWizard_vitePitfall()}</p>
    {/if}
    {#if questions.proxy_rails_pitfall}
      <p class="text-xs text-amber-600 dark:text-amber-500">{m.siteWizard_railsPitfall()}</p>
    {/if}
    </div>
  {/if}

  {#if answers.kind === 'container'}
    <div class="space-y-4" transition:slide={{ duration: 160 }}>
    <WizardField label={m.siteWizard_port()} hint={m.siteWizard_containerPortHint()}>
      <input
        type="number"
        value={answers.container_port ?? 0}
        oninput={(e) => set({ container_port: port((e.currentTarget as HTMLInputElement).value) })}
        class="w-full text-sm bg-white dark:bg-lerd-bg border border-gray-200 dark:border-lerd-border rounded-sm px-2 py-1.5 text-gray-700 dark:text-gray-300 focus:outline-hidden focus:border-lerd-red/50"
      />
    </WizardField>

    <WizardField label={m.siteWizard_containerfile()}>
      <input
        type="text"
        value={answers.containerfile ?? ''}
        oninput={(e) => set({ containerfile: (e.currentTarget as HTMLInputElement).value })}
        class="w-full text-sm bg-white dark:bg-lerd-bg border border-gray-200 dark:border-lerd-border rounded-sm px-2 py-1.5 text-gray-700 dark:text-gray-300 focus:outline-hidden focus:border-lerd-red/50"
      />
    </WizardField>
    </div>
  {/if}

  {#if questions.https_available}
    <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300 cursor-pointer">
      <input
        type="checkbox"
        checked={answers.secured}
        onchange={(e) => set({ secured: (e.currentTarget as HTMLInputElement).checked })}
        class="rounded-sm border-gray-300 dark:border-lerd-border"
      />
      {m.siteWizard_https()}
    </label>
  {/if}

  {#if (questions.database_options ?? []).length > 0}
    <WizardField label={m.siteWizard_database()}>
      <Dropdown
        value={answers.database ?? ''}
        width="full"
        options={(questions.database_options ?? []).map((o) => ({ value: o.value, label: o.label }))}
        onchange={(v) => set({ database: v })}
      />
    </WizardField>
  {/if}

  {#if serviceItems.length > 0}
    <WizardField label={m.siteWizard_services()}>
      <WizardCheckList
        items={serviceItems}
        selected={answers.services}
        onchange={(selected) => set({ services: selected })}
      />
    </WizardField>
  {/if}

  {#if answers.kind === 'php' && questions.frankenphp_offered}
    <WizardField label={m.siteWizard_frankenphp()} hint={questions.frankenphp_reason}>
      <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300 cursor-pointer">
        <input
          type="checkbox"
          checked={answers.frankenphp ?? false}
          onchange={(e) => set({ frankenphp: (e.currentTarget as HTMLInputElement).checked })}
          class="rounded-sm border-gray-300 dark:border-lerd-border"
        />
        {m.siteWizard_frankenphpUse()}
      </label>
      {#if answers.frankenphp}
        <label
          class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300 cursor-pointer"
          transition:slide={{ duration: 140 }}
        >
          <input
            type="checkbox"
            checked={answers.frankenphp_worker ?? false}
            onchange={(e) =>
              set({ frankenphp_worker: (e.currentTarget as HTMLInputElement).checked })}
            class="rounded-sm border-gray-300 dark:border-lerd-border"
          />
          {m.siteWizard_frankenphpWorker()}
        </label>
      {/if}
    </WizardField>
  {/if}

  {#if answers.kind === 'php' && workerItems.length > 0}
    <WizardField label={m.siteWizard_workers()} hint={m.siteWizard_workersHint()}>
      <WizardCheckList
        items={workerItems}
        selected={answers.workers}
        onchange={(selected) => set({ workers: selected })}
      />
    </WizardField>
  {/if}
</div>
