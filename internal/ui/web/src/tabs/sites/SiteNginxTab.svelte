<script lang="ts">
  import NginxEditor from '$components/NginxEditor.svelte';
  import ConfigToolbar from '$components/ConfigToolbar.svelte';
  import ConfigFileBody from '$components/ConfigFileBody.svelte';
  import { ConfigFile } from '$lib/configFile.svelte';
  import {
    getSiteNginx,
    loadSiteNginxBackups,
    loadSiteNginxBackupContent,
    type NginxScope,
    type Site
  } from '$stores/sites';
  import { openNginxSaveModal, openNginxRestoreModal, openNginxResetModal } from '$stores/modals';
  import { onMount } from 'svelte';

  interface Props {
    site: Site;
    /** Domain whose override to edit. Defaults to the site's primary domain;
     *  pass a worktree's domain to edit that worktree's override instead. */
    domain?: string;
    /** Which override file to edit: the server block one, or the one included
     *  inside the block that serves the site. */
    scope?: NginxScope;
    /** Called after a successful save so the host can close the editor. */
    onSaved?: () => void;
  }
  let { site, domain, scope = 'server', onSaved = () => {} }: Props = $props();

  const file = new ConfigFile();

  // The domain is the only reactive input that should re-load the editor.
  // Every site mutation broadcasts a fresh site object, even when the domain
  // is unchanged; pinning to a $derived string keeps those from clobbering edits.
  const currentDomain = $derived(domain || site.domain);

  $effect(() => {
    const d = currentDomain;
    const s = scope;
    void file.load({
      get: () => getSiteNginx(d, s),
      backups: () => loadSiteNginxBackups(d, s),
      backup: (name) => loadSiteNginxBackupContent(d, s, name)
    });
  });

  onMount(() => () => file.dispose());

  // The diff baseline is the saved bytes, not the buffer, so the preview
  // matches exactly what the backend will write.
  function restore() {
    const d = currentDomain;
    void file.restore((backupName, backup, done) =>
      openNginxRestoreModal({ domain: d, scope, current: file.original, backupName, backup }, done)
    );
  }

  function reset() {
    openNginxResetModal({ domain: currentDomain, scope, path: file.path }, file.refreshLater());
  }

  // On success the host closes the editor; reopening reloads the saved file,
  // so there is no state left to refresh here.
  function save() {
    openNginxSaveModal(
      { domain: currentDomain, scope, content: file.text, original: file.original, exists: file.exists },
      () => onSaved()
    );
  }
</script>

<div class="flex flex-col h-full">
  <ConfigToolbar {file} onReset={reset} onRestore={restore} onSave={save} />

  <ConfigFileBody {file}>
    <NginxEditor bind:value={file.text} language={scope === 'web' ? 'yaml' : 'nginx'} />
  </ConfigFileBody>
</div>
