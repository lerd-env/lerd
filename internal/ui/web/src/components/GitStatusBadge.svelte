<script lang="ts">
  import { gitParts, gitMarker, type GitStatus, type GitPartKind } from '$lib/gitStatus';
  import { tooltip } from '$lib/tooltip';
  import { m } from '../paraglide/messages.js';

  let { status }: { status: GitStatus } = $props();

  const marker = $derived(gitMarker(status));

  function describe(kind: GitPartKind, count: number): string {
    switch (kind) {
      case 'conflicted':
        return m.gitStatus_conflicted({ count });
      case 'untracked':
        return m.gitStatus_untracked({ count });
      case 'modified':
        return m.gitStatus_modified({ count });
      case 'staged':
        return m.gitStatus_staged({ count });
      case 'ahead':
        return m.gitStatus_ahead({ count });
      case 'behind':
        return m.gitStatus_behind({ count });
    }
  }

  const label = $derived(gitParts(status).map((p) => describe(p.kind, p.count)).join(' · '));
</script>

{#if marker.text}
  <span
    class="font-mono text-xs leading-none -ml-1 {marker.conflicted ? 'text-red-600 dark:text-red-400' : 'text-gray-400 dark:text-gray-500'}"
    use:tooltip={label}
    aria-label={label}
    role="img">{marker.text}</span
  >
{/if}
