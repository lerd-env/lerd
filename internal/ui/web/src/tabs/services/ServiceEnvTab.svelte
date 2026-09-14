<script lang="ts">
  import EnvBlock from '$components/EnvBlock.svelte';
  import CopyButton from '$components/CopyButton.svelte';
  import type { Service } from '$stores/services';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    svc: Service;
  }
  let { svc }: Props = $props();
</script>

<div class="p-3 sm:p-5 space-y-4 overflow-y-auto">
  {#if svc.connection_url}
    <div class="rounded-xl border border-gray-200/80 dark:border-lerd-border bg-white dark:bg-lerd-card px-4 py-3 space-y-2">
      <span class="text-sm font-medium text-gray-800 dark:text-gray-200">{m.services_env_connect()}</span>
      <div class="flex items-start gap-3">
        <a
          href={svc.connection_url}
          class="min-w-0 flex-1 break-all font-mono text-xs text-sky-600 dark:text-sky-400 hover:underline"
        >{svc.connection_url}</a>
        <CopyButton text={svc.connection_url} label={m.common_copy()} class="mt-0.5" />
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400">
        {@html m.services_env_connectHint({ loopback4: '<code class="font-mono text-gray-600 dark:text-gray-300">127.0.0.1</code>', loopback6: '<code class="font-mono text-gray-600 dark:text-gray-300">localhost</code>' })}
      </p>
    </div>
  {/if}
  {#if svc.env_vars && Object.keys(svc.env_vars).length > 0}
    <EnvBlock vars={svc.env_vars} />
  {:else}
    <p class="text-sm text-gray-400 dark:text-gray-500">{m.services_env_none()}</p>
  {/if}
</div>
