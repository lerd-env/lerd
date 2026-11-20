<script lang="ts">
  import NginxEditor from '$components/NginxEditor.svelte';
  import ConfigToolbar from '$components/ConfigToolbar.svelte';
  import ConfigFileBody from '$components/ConfigFileBody.svelte';
  import { ConfigFile } from '$lib/configFile.svelte';
  import {
    getNginxConfig,
    loadNginxConfigBackups,
    loadNginxConfigBackupContent,
    type GlobalConfigTarget
  } from '$stores/nginx';
  import {
    openNginxGlobalSaveModal,
    openNginxGlobalRestoreModal,
    openNginxGlobalResetModal
  } from '$stores/modals';
  import { onMount } from 'svelte';

  interface Props {
    /** 'web' edits lerd's own web server's settings for every site. */
    target?: GlobalConfigTarget;
  }
  let { target = 'nginx' }: Props = $props();

  const file = new ConfigFile();

  onMount(() => {
    void file.load({
      get: () => getNginxConfig(target),
      backups: () => loadNginxConfigBackups(target),
      backup: (name) => loadNginxConfigBackupContent(name, target)
    });
    return () => file.dispose();
  });

  function restore() {
    void file.restore((backupName, backup, done) =>
      openNginxGlobalRestoreModal({ target, current: file.original, backupName, backup }, done)
    );
  }

  function reset() {
    openNginxGlobalResetModal({ target, path: file.path }, file.refreshLater());
  }

  function save() {
    openNginxGlobalSaveModal(
      { target, content: file.text, original: file.original, exists: file.exists },
      file.refreshLater()
    );
  }
</script>

<div class="flex flex-col h-full">
  <ConfigToolbar {file} onReset={reset} onRestore={restore} onSave={save} />

  <ConfigFileBody {file}>
    <NginxEditor bind:value={file.text} language={target === 'web' ? 'yaml' : 'nginx'} />
  </ConfigFileBody>
</div>
