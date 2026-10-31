<script lang="ts">
  import PluginArt from './PluginArt.svelte';
  import PluginGlyph, { type GlyphName } from './PluginGlyph.svelte';
  import PluginNewCommand from './PluginNewCommand.svelte';
  import Icon from '$components/Icon.svelte';
  import { goToTab } from '$stores/route';
  import { m } from '../../paraglide/messages.js';

  // What plugins add and the two ways to get one, the catalog or a plugin of
  // your own: the whole Installed view before any plugin is. A folder or git
  // URL is the third way, kept small.
  let { oninstall }: { oninstall: () => void } = $props();
  const surfaces: [GlyphName, () => string][] = [
    ['terminal', m.system_plugins_chipCommands],
    ['page', m.system_plugins_chipPages],
    ['tab', m.system_plugins_chipTabs],
    ['card', m.system_plugins_chipCards],
    ['sparkle', m.system_plugins_chipAi],
    ['database', m.system_plugins_chipServices]
  ];
  const CARD = 'flex flex-col gap-2.5 rounded-xl border border-gray-200/80 dark:border-lerd-border bg-white dark:bg-lerd-card p-4 text-left';
  const ICON = 'shrink-0 flex items-center justify-center w-10 h-10 rounded-lg bg-lerd-red/10 text-lerd-red';
</script>

<div class="w-full mx-auto max-w-3xl pt-4 pb-6 text-center">
  <div class="flex justify-center"><PluginArt kind="installed" /></div>
  <h2 class="mt-4 text-xl font-semibold tracking-tight text-gray-900 dark:text-white">{m.system_plugins_emptyTitle()}</h2>
  <p class="mx-auto mt-1.5 max-w-lg text-sm text-gray-500 dark:text-gray-400">{m.system_plugins_emptyLead()}</p>
  <ul class="mt-4 flex flex-wrap justify-center gap-2">
    {#each surfaces as [glyph, label] (glyph)}
      <li class="inline-flex items-center gap-1.5 rounded-full border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card px-2.5 py-1 text-xs text-gray-600 dark:text-gray-300">
        <PluginGlyph name={glyph} class="w-3.5 h-3.5 text-lerd-red" />{label()}
      </li>
    {/each}
  </ul>
  <div class="mt-6 grid gap-3 sm:grid-cols-2">
    <section class={CARD}>
      <div class="flex items-start gap-3">
        <span class={ICON}><PluginGlyph name="download" class="w-5 h-5" /></span>
        <div>
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{m.system_plugins_browseTitle()}</h3>
          <p class="text-xs text-gray-500 dark:text-gray-400">{m.system_plugins_browseBody()}</p>
        </div>
      </div>
      <div class="mt-auto pt-1">
        <button type="button" onclick={() => goToTab('plugins', 'browse')}
          class="inline-flex items-center gap-1.5 h-8 px-3 rounded-md bg-lerd-red hover:bg-lerd-redhov text-lerd-onred text-xs font-semibold">
          <PluginGlyph name="download" class="w-3.5 h-3.5" />{m.system_plugins_browseAction()}
        </button>
      </div>
    </section>
    <section class={CARD}>
      <div class="flex items-start gap-3">
        <span class={ICON}><PluginGlyph name="code" class="w-5 h-5" /></span>
        <div>
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{m.system_plugins_buildTitle()}</h3>
          <p class="text-xs text-gray-500 dark:text-gray-400">{m.system_plugins_buildBody()}</p>
        </div>
      </div>
      <PluginNewCommand />
      <p class="flex items-start gap-1.5 text-xs text-gray-500 dark:text-gray-400">
        <PluginGlyph name="sparkle" class="mt-0.5 w-3.5 h-3.5 shrink-0 text-lerd-red" />{m.system_plugins_buildAi()}
      </p>
    </section>
  </div>
  <button type="button" onclick={oninstall} class="mt-4 inline-flex items-center gap-1 text-xs font-medium text-lerd-red hover:text-lerd-redhov">
    <Icon name="plus" class="w-3.5 h-3.5" />{m.system_plugins_installFrom()}
  </button>
</div>
