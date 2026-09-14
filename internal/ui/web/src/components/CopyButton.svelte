<script lang="ts">
  import { onDestroy } from 'svelte';
  import { tooltip } from '$lib/tooltip';
  import { copyText } from '$lib/clipboard';
  import Icon from './Icon.svelte';
  import { m } from '../paraglide/messages.js';

  interface Props {
    // A thunk defers work that only matters on click, such as inlining bindings
    // into a statement for every row of a long list.
    text: string | (() => string);
    label: string;
    class?: string;
    size?: string;
    // faint keeps the button quiet where it sits beside a control of its own,
    // so it doesn't read as part of that control.
    tone?: 'default' | 'faint';
  }
  let { text, label, class: cls = '', size = 'w-3.5 h-3.5', tone = 'default' }: Props = $props();

  const idle = $derived(
    tone === 'faint'
      ? 'text-gray-300 dark:text-gray-600 hover:text-gray-600 dark:hover:text-gray-200'
      : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-200'
  );

  let copied = $state(false);
  let failed = $state(false);
  let timer: ReturnType<typeof setTimeout> | null = null;
  onDestroy(() => {
    if (timer) clearTimeout(timer);
  });

  async function copy() {
    const ok = await copyText(typeof text === 'function' ? text() : text);
    // A copy that did not happen says so, rather than leaving a stale clipboard
    // to be discovered at the paste.
    copied = ok;
    failed = !ok;
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      copied = false;
      failed = false;
    }, 1500);
  }
</script>

<button
  type="button"
  class="shrink-0 flex items-center {copied
    ? 'text-emerald-600 dark:text-emerald-500'
    : failed
      ? 'text-red-500 dark:text-red-400'
      : idle} {cls}"
  onclick={copy}
  use:tooltip={failed ? m.common_failed() : label}
  aria-label={label}
>
  <Icon name={copied ? 'check' : failed ? 'alert' : 'clipboard'} class={size} />
</button>
