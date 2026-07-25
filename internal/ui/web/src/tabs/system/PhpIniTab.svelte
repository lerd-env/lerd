<script lang="ts">
  import PhpIniPane from './PhpIniPane.svelte';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    version: string;
  }
  let { version }: Props = $props();
</script>

<!--
  The two files work together: the shared ini sets the baseline and the version
  ini overrides it, so both are on screen at once. Each pane owns its own state,
  so saving one never disturbs unsaved edits in the other. Panes stack below lg.
-->
<div class="flex flex-col lg:flex-row h-full min-h-0 overflow-y-auto lg:overflow-visible">
  <PhpIniPane
    scope={version}
    title={'PHP ' + version}
    hint={m.system_php_iniScopeVersion()}
    label={'PHP ' + version}
  />
  <div class="border-t lg:border-t-0 lg:border-l border-gray-100 dark:border-lerd-border"></div>
  <PhpIniPane
    scope="shared"
    title={m.system_php_iniScopeSharedLabel()}
    hint={m.system_php_iniScopeSharedDesc()}
    label={m.system_php_sharedScopeLabel()}
  />
</div>
