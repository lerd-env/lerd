<script lang="ts">
  import { openGitDiffModal } from '$stores/modals';
  import { tooltip } from '$lib/tooltip';
  import { m } from '../paraglide/messages.js';

  interface Props {
    domain: string;
    // The checkout's folder as git-status reported it.
    path: string;
    branchLabel: string;
  }
  let { domain, path, branchLabel }: Props = $props();
</script>

<button
  type="button"
  onclick={() => openGitDiffModal(domain, path, branchLabel)}
  use:tooltip={m.gitDiff_open()}
  aria-label={m.gitDiff_open()}
  class="h-8 min-w-8 px-1.5 flex items-center justify-center rounded-md transition-colors text-gray-500 dark:text-gray-400 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5"
>
  <svg class="w-4 h-4 shrink-0" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" viewBox="0 0 24 24">
    <path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z M14 3v5h5 M12 10v6 M9 13h6 M9 18h6" />
  </svg>
</button>
