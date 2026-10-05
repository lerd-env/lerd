<script lang="ts">
  import ButtonMenu, { type ButtonMenuAction } from './ButtonMenu.svelte';

  interface Props {
    actions: ButtonMenuAction[];
    busy?: boolean;
    onSettings?: () => void;
    settingsTitle?: string;
    inline?: ButtonMenuAction;
  }
  let { actions, busy = false, onSettings, settingsTitle, inline }: Props = $props();
</script>

<ButtonMenu {actions} {busy} {onSettings} {settingsTitle} {inline} />
