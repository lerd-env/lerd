<script lang="ts">
  import Icon from '$components/Icon.svelte';
  import { tooltip } from '$lib/tooltip';

  interface Props {
    label: string;
    disabled?: boolean;
    onclick: () => void;
  }
  let { label, disabled = false, onclick }: Props = $props();
</script>

<!-- Sits on the card's top-right corner like a window close, out of sight until
     the card is hovered or the button focused, so it never competes with the
     card's real actions. Touch screens have no hover, so it stays visible there. -->
<button
  type="button"
  {disabled}
  {onclick}
  use:tooltip={label}
  aria-label={label}
  class="absolute -top-[7px] -right-[7px] z-10 flex items-center justify-center w-4.5 h-4.5 rounded-full border border-gray-300 dark:border-white/25 bg-white dark:bg-lerd-card text-gray-600 dark:text-gray-100 shadow opacity-0 group-hover:opacity-100 focus-visible:opacity-100 [@media(hover:none)]:opacity-100 hover:bg-lerd-red hover:border-lerd-red hover:text-white transition duration-300 disabled:opacity-50"
>
  <Icon name="close" class="w-2.5 h-2.5" />
</button>
