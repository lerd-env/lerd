<script lang="ts">
  import Badge from '$components/Badge.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import { fetchPhpExtensions, type PhpExtensionsReport, type PhpSetState } from '$stores/phpVersions';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    version: string;
  }
  let { version }: Props = $props();

  let report = $state<PhpExtensionsReport | null>(null);
  let loading = $state(true);
  let error = $state('');

  // Reading php -m starts a container, so this loads on open rather than on
  // every status broadcast. The backend caches it against the image ID.
  $effect(() => {
    const v = version;
    loading = true;
    error = '';
    fetchPhpExtensions(v).then((res) => {
      if (v !== version) return; // a newer version won the race
      report = res.report ?? null;
      error = res.error ?? res.modules_error ?? '';
      loading = false;
    });
  });

  const declaredCount = $derived(
    (report?.extensions.declared?.length ?? 0) + (report?.packages.declared?.length ?? 0)
  );
  const modules = $derived(report?.modules ?? []);

  // A declared entry is only shown as present when the image really has it, so
  // nothing here advertises what the image did not load.
  function entries(set: PhpSetState): { name: string; has: boolean }[] {
    return (set.declared ?? []).map((name) => ({ name, has: (set.has ?? []).includes(name) }));
  }
</script>

<div class="flex flex-col h-full">
  <div class="flex-1 overflow-y-auto p-3 sm:p-5 space-y-5">
    {#if loading}
      <p class="text-sm text-gray-400">…</p>
    {:else if !report?.built}
      <EmptyState title={m.system_php_ext_notBuilt({ version })} />
    {:else}
      {#if report.needs_rebuild}
        <p
          class="text-xs text-amber-700 dark:text-amber-400 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-500/30 rounded-lg px-3 py-2"
        >
          {m.system_php_ext_needsRebuild({ version })}
        </p>
      {/if}

      {#if declaredCount === 0}
        <p class="text-xs text-gray-500 dark:text-gray-400">{m.system_php_ext_none()}</p>
      {:else}
        {#each [{ label: m.system_php_ext_declared(), set: report.extensions }, { label: m.system_php_ext_packages(), set: report.packages }] as group (group.label)}
          {#if (group.set.declared?.length ?? 0) > 0}
            <div class="space-y-2">
              <span class="text-sm font-medium text-gray-800 dark:text-gray-200">{group.label}</span>
              <div class="flex flex-wrap gap-1.5">
                {#each entries(group.set) as e (e.name)}
                  <Badge
                    tone={e.has ? 'running' : 'stopped'}
                    dot={!report.needs_rebuild}
                    title={e.has ? undefined : m.system_php_ext_cannot({ version })}
                  >
                    {e.name}
                  </Badge>
                {/each}
              </div>
            </div>
          {/if}
        {/each}
        <p class="text-xs text-gray-500 dark:text-gray-400 -mt-2">{m.system_php_ext_manageHelp()}</p>
      {/if}

      <div class="space-y-2">
        <span class="text-sm font-medium text-gray-800 dark:text-gray-200">
          {m.system_php_ext_modules({ count: modules.length })}
        </span>
        <p class="text-xs text-gray-500 dark:text-gray-400">{m.system_php_ext_modulesHelp({ version })}</p>
        <div class="flex flex-wrap gap-1.5">
          {#each modules as mod (mod)}
            <Badge tone="neutral">{mod}</Badge>
          {/each}
        </div>
      </div>

      {#if error}
        <p class="text-xs text-red-500">{error}</p>
      {/if}
    {/if}
  </div>
</div>
