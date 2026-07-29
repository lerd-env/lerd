<script lang="ts">
  import Icon from '$components/Icon.svelte';

  interface Props {
    title: string;
    testid: string;
    onclick: () => void;
  }
  let { title, testid, onclick }: Props = $props();
</script>

<button
  type="button"
  role="menuitem"
  {title}
  aria-label={title}
  data-testid={testid}
  {onclick}
  class="shrink-0 px-2.5 flex items-center text-gray-400 hover:text-lerd-red dark:text-gray-500 dark:hover:text-lerd-red transition-colors"
>
  <Icon name="system" class="w-3.5 h-3.5" />
</button>
