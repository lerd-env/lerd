<script lang="ts">
  import { onMount } from 'svelte';
  import { slide } from 'svelte/transition';
  import Dropdown from '$components/Dropdown.svelte';
  import FrameworkMark from '$components/FrameworkMark.svelte';
  import WizardField from './WizardField.svelte';
  import { slugifyProjectName } from '$lib/projectName';
  import { frameworkCatalogue, type FrameworkChoice } from '$stores/wizard';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    parent: string;
    name: string;
    framework: string;
    frameworkVersion: string;
    onchange: (v: { name: string; framework: string; frameworkVersion: string }) => void;
    onerror?: (message: string) => void;
  }
  let { parent, name, framework, frameworkVersion, onchange, onerror }: Props = $props();

  let catalogue = $state<FrameworkChoice[]>([]);
  let loading = $state(false);

  const chosen = $derived(catalogue.find((f) => f.name === framework));
  // Latest is the answer unless an older major is picked, so the version list
  // leads with it rather than making the newest project the odd one out.
  const versionOptions = $derived([
    { value: '', label: m.siteWizard_versionLatest() },
    ...(chosen?.versions ?? []).map((v) => ({ value: v, label: v }))
  ]);

  function emit(next: Partial<{ name: string; framework: string; frameworkVersion: string }>) {
    onchange({ name, framework, frameworkVersion, ...next });
  }

  onMount(async () => {
    loading = true;
    try {
      catalogue = await frameworkCatalogue();
      if (!framework && catalogue.length > 0) emit({ framework: catalogue[0].name });
    } catch (e) {
      onerror?.(e instanceof Error ? e.message : m.common_failed());
    } finally {
      loading = false;
    }
  });
</script>

<div class="px-5 py-4 space-y-4">
  <WizardField label={m.siteWizard_parentDirectory()}>
    <div class="text-sm font-mono text-gray-700 dark:text-gray-300 truncate">{parent}</div>
  </WizardField>

  <WizardField label={m.siteWizard_projectName()}>
    <input
      type="text"
      value={name}
      oninput={(e) => {
        const input = e.currentTarget as HTMLInputElement;
        const slug = slugifyProjectName(input.value);
        input.value = slug;
        emit({ name: slug });
      }}
      placeholder={m.siteWizard_projectNamePlaceholder()}
      class="w-full text-sm bg-white dark:bg-lerd-bg border border-gray-200 dark:border-lerd-border rounded-sm px-2 py-1.5 text-gray-700 dark:text-gray-300 focus:outline-hidden focus:border-lerd-red/50"
    />
  </WizardField>

  <WizardField label={m.siteWizard_framework()}>
    <Dropdown
      value={framework}
      width="full"
      disabled={loading || catalogue.length === 0}
      placeholder={loading ? m.common_loading() : ''}
      options={catalogue.map((f) => ({ value: f.name, label: f.label }))}
      onchange={(v) => emit({ framework: v, frameworkVersion: '' })}
    >
      {#snippet optionIcon(name)}
        <FrameworkMark {name} size="md" />
      {/snippet}
    </Dropdown>
  </WizardField>

  {#if (chosen?.versions ?? []).length > 0}
    <div transition:slide={{ duration: 160 }}>
      <WizardField label={m.siteWizard_version()}>
        <Dropdown
          value={frameworkVersion}
          width="full"
          options={versionOptions}
          onchange={(v) => emit({ frameworkVersion: v })}
        />
      </WizardField>
    </div>
  {/if}
</div>
