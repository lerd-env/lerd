<script lang="ts">
  import ThemeSwitcher from './ThemeSwitcher.svelte';
  import NotificationCenter from './NotificationCenter.svelte';
  import VersionLabel from './VersionLabel.svelte';
</script>

<div
  class="md:hidden flex items-center justify-between px-3 py-3 border-b border-gray-100 dark:border-lerd-border sticky top-0 bg-white dark:bg-lerd-card z-10"
>
  <div class="flex items-center gap-2">
    <img src="/icons/icon.svg" class="w-6 h-6 rounded-md" alt="Lerd" />
    <span class="font-semibold text-sm text-gray-900 dark:text-white">Lerd</span>
    <VersionLabel size="header" />
  </div>
  <div class="flex items-center gap-1">
    <NotificationCenter size="md" align="right" />
    <ThemeSwitcher size="md" align="right" />
  </div>
</div>
