# Histórico de Releases — Lerd Oracle Edition

Atalho: <https://github.com/gabriel-sousa99/lerd/releases>

Esquema de versão: `v1.21.2-oracle.N` onde `N` cresce a cada release da fork. O fork foi cortado do `geodro/lerd@v1.21.2`; o auto-update (`lerd update`) consulta este repo.

## v1.21.2-oracle.13 — Pin do `.php-version` absoluto
**Bug fix.** Dropdown PHP voltava pra versão antiga após reload porque `enrichVersions` clampava `.php-version=7.4` contra o `fw.PHP.Min/Max` do framework bundled (Laravel 13, requer 8.4+). Agora `readUserPHPPin()` bypassa o clamp quando o user pinou explicitamente. Projetos sem pin mantêm o auto-clamp upstream.

## v1.21.2-oracle.12 — SSH no container
**Bug fix.** `composer update` com deps via `ssh://git@…` falhava com "Permission denied (publickey)" mesmo com `openssh-client` instalado, porque git/ssh procuravam chaves em `/root/.ssh` (HOME do container) e não em `/home/gabriel/.ssh`. Quadlet PHP agora monta `$HOME/.ssh:/root/.ssh:ro` read-only + `GIT_SSH_COMMAND=ssh -o StrictHostKeyChecking=accept-new`.

## v1.21.2-oracle.11 — Site PHP persistence + Laravel version chip + start/stop indicator
**3 bug fixes.**
- Trocar versão PHP no dropdown não persistia em sites.yaml (só FrankenPHP chamava `AddSite`).
- Chip "Laravel 13" num projeto Laravel 8: `frameworkLabel` usava `fw.Version` (latest bundled) em vez de `DetectMajorVersion(composer.json)`.
- Botões start/stop PHP sem feedback visível: agora label muda pra "Iniciando…"/"Parando…" + banner inline com dot pulsante.

## v1.21.2-oracle.10 — Editores de .env + PHP install streaming
**4 features.**
- **Site .env editor**: textarea com Save/Discard/Ctrl+S, backup `.env.before_lerd` automático, beforeunload prompt.
- **Service env editor**: key=value rows pro bloco `Environment=` do quadlet, source badge ("preset" vs "override").
- **PHP install com SSE logs**: stream do `podman build` linha-a-linha no dashboard, beforeunload guard durante o build.
- **openssh-client** no Containerfile.

## v1.21.2-oracle.9 — Xdebug silencioso
**Bug fix.** `xdebug.start_with_request=yes` causava spam "Could not connect to debugging client" em todo comando CLI. Default trocado pra `=trigger` (debug só dispara via cookie/header `XDEBUG_TRIGGER` ou query param).

## v1.21.2-oracle.8 — oracle-xe rootless + typesense-dashboard
**Bug fix + feature.**
- `oracle-xe` preset ganha `userns: keep-id:uid=54321,gid=54321` + `chown_data: true` pra funcionar rootless (oracle uid 54321 ≠ host user 1000).
- `default_version: 21-slim-faststart` (snapshot prébuild, ~30s startup) em vez de `21-slim` (cold init ~10min).
- Novo preset `typesense-dashboard` (bfritscher/typesense-dashboard) na porta 8109.

## v1.21.2-oracle.7 — Comandos Laravel customizados + filtro destrutivo
**Feature.**
- Dashboard auto-descobre comandos artisan em `app/Console/Commands/*.php` via regex de `$signature`/`$description` (sem rodar PHP).
- Deny-list de destrutivos em 2 camadas: filtra da list E retorna HTTP 403 na execução. Cobre `migrate:fresh`, `db:wipe`, `schema:drop`, `doctrine:fixtures:load`, `queue:flush`, `DROP TABLE`, `rm -rf /`, etc.

## v1.21.2-oracle.6 — Botão VS Code + DB no link + oracle-xe/typesense presets
**3 features.**
- Botão "Abrir no editor" ao lado do terminal (code, code-insiders, codium, cursor, phpstorm, webstorm, idea, goland, subl, zed, nova; macOS via `open -a`).
- `lerd link` agora pergunta DB_CONNECTION quando `.lerd.yaml` não tem Services.
- Presets `oracle-xe` (3 variantes: slim-faststart/slim/full) + `typesense` (search engine).

## v1.21.2-oracle.5 — PHP 5.6 + .localhost default + tray rebrand
**Feature + change.**
- PHP 5.6 (Legacy estendida): Alpine 3.8 base, oci8 2.0.12, xdebug 2.5.5, redis 4.3.0, symlink `libresolv.so.2` pro Oracle Instant Client. Sem memcached/amqp/pcov/spx.
- Default DNS é off, TLD é `.localhost`. Existing installs mantêm o que estava.
- Tray tooltip: "Lerd Oracle Edition" + entrada "Debug & Oracle help…" abre `docs/DEBUG.md` no browser.

## v1.21.2-oracle.4 — Debug area + PHP install picker + remove destructive defaults
**Feature.**
- `docs/DEBUG.md` + 9 sub-arquivos (`podman`, `nginx`, `dns`, `php-fpm`, `oracle`, `sites`, `services`, `workers`, `updates`) com formato 🔴 sintoma / 🔍 diagnóstico / 🟢 conserto.
- Dashboard `System → Debug & Troubleshoot` com botões de diagnóstico (`lerd doctor`, `dns:check`, etc.) + cards pros guias.
- `System → Instalar versão…` lista versões PHP não instaladas (7.4 → 8.5).
- Removidos `migrate:fresh` e `doctrine:fixtures:load` dos defaults dos frameworks Laravel/Symfony.

## v1.21.2-oracle.2 — Dashboard: gerenciar extensões PHP
**Feature.** `System → PHP X.Y → Extensões customizadas`: chips de preset (imap, swoole, ssh2, apcu, event, pspell, tidy, pdo_dblib) + form livre com `apk-deps`. Backend: `GET/POST/DELETE /api/php-versions/{v}/extensions[/{ext}]`. Equivalente a `lerd php:ext add`.

## v1.21.2-oracle.1 — Oracle baked-in + identidade da fork
**Feature inicial.**
- Containerfile patch: Oracle Instant Client 21.18 (Basic + SDK) baixado e descomprimido no builder; `oci8` compilado via PECL com pinning por PHP major (2.2.0 → 3.4.1).
- `lerd init` ganha "Oracle" como 4ª opção de Database, com sub-form pra host/port/service/user/pass salvo em `.lerd.yaml`.
- Comparador `oracle.N` (não-prerelease) pra que `lerd update` reconheça releases da fork como upgrades. Auto-update aponta pra `gabriel-sousa99/lerd/releases`.
- Tooling: README pt-BR, defaults `.localhost`, `memcached` + `amqp` PECL pré-instalados.
