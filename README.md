# Lerd Oracle Edition

> Fork do [`geodro/lerd`](https://github.com/geodro/lerd) com **suporte a
> Oracle Database embutido em todas as imagens PHP** — Oracle Instant
> Client 21.18 (LTS) + `oci8` + memcached + amqp já compilados, prontos
> para PHP 7.4 → 8.5. Drop-in replacement: todo comando `lerd` existente
> funciona igual.

> [!IMPORTANT]
> Este fork mantém o mesmo binário `lerd` (compatibilidade total) e
> aponta o auto-update para **este** repositório em vez do upstream.
> Releases publicadas aqui seguem o esquema `v1.21.2-oracle.N`.

[![Fork base](https://img.shields.io/badge/forked%20from-geodro%2Flerd%20v1.21.2-blue)](https://github.com/geodro/lerd)
[![Oracle Instant Client](https://img.shields.io/badge/Oracle%20Instant%20Client-21.18-red)]()
[![PHP](https://img.shields.io/badge/PHP-7.4%20%E2%80%93%208.5-777BB4)]()

---

## Sumário

- [O que muda em relação ao upstream](#o-que-muda-em-relação-ao-upstream)
- [Instalação](#instalação)
- [Instalando no WSL2 (Ubuntu/Debian)](#instalando-no-wsl2-ubuntudebian)
- [Primeiro uso](#primeiro-uso)
- [Trabalhando com Oracle](#trabalhando-com-oracle)
- [Serviços (start, stop, status)](#serviços-start-stop-status)
- [Rebuild de imagens PHP](#rebuild-de-imagens-php)
- [Atualização](#atualização)
- [Desinstalação](#desinstalação)
- [Diagnóstico](#diagnóstico)
- [Recursos exclusivos do dashboard](#recursos-exclusivos-do-dashboard)
- [Debug e troubleshooting](#debug-e-troubleshooting)
- [Lista de comandos](#lista-de-comandos-úteis)

---

## O que muda em relação ao upstream

| Recurso                                  | Upstream `geodro/lerd`         | Esta fork                                                          |
|------------------------------------------|--------------------------------|--------------------------------------------------------------------|
| Driver Oracle (`oci8`)                   | precisa instalar manual         | **compilado em toda imagem** (`oci8` 2.0.12 → 3.4.1 por versão PHP) |
| Oracle Instant Client                    | n/a                            | **21.18 LTS** em `/opt/oracle/instantclient`                       |
| Extensão `memcached`                     | precisa `lerd php:ext`         | **pré-instalada**                                                  |
| Extensão `amqp` (RabbitMQ)               | precisa `lerd php:ext`         | **pré-instalada**                                                  |
| `openssh-client` no container            | ausente (composer ssh falha)    | **instalado** + `$HOME/.ssh` montado em `/root/.ssh`               |
| Suporte PHP                              | 7.4 → 8.5                      | **5.6 → 8.5** (5.6 legacy estendida com libresolv shim)            |
| `lerd init` → "Database"                 | sqlite / mysql / postgres      | **+ Oracle (externo)**                                             |
| `lerd link` pergunta DB                  | só `lerd init`                 | **também ao `link` se .lerd.yaml não tem DB**                      |
| `.lerd.yaml` → bloco `oracle:`           | ✗                              | **host/port/service/user/pass/charset**                            |
| DNS padrão                               | `lerd-dns` + `.test` (sudo)    | **off, `.localhost`**                                              |
| Comandos destrutivos no dashboard        | `migrate:fresh` etc. um clique | **filtrados em 2 camadas** (lista + run-time HTTP 403)             |
| Comandos artisan customizados            | só os do framework             | **auto-discovery de `app/Console/Commands/*.php`**                 |
| Editor de `.env` no dashboard            | só leitura                     | **editor textarea com Save/Discard/Ctrl+S + backup auto**          |
| Editor de env do serviço no dashboard    | só leitura                     | **editor key=value com restart hint**                              |
| Instalar nova versão PHP                 | só CLI                         | **botão no dashboard + SSE logs ao vivo + beforeunload guard**     |
| Botão "Abrir no editor" no site          | só terminal                    | **+ editor (code/cursor/phpstorm/…)**                              |
| Service presets adicionais               | mysql/postgres/redis/…         | **+ `oracle-xe` + `typesense` + `typesense-dashboard`**            |
| Auto-update                              | `geodro/lerd/releases`         | `gabriel-sousa99/lerd/releases`                                    |
| Versão                                   | `1.21.2`                       | `1.21.2-oracle.N`                                                  |
| Xdebug por padrão                        | `start_with_request=yes`       | **`=trigger`** (sem spam em CLI sem IDE)                           |

### Lista completa de extensões PHP nas imagens

> 25 extensões compiladas no builder + 7 via PECL — cobre o ecossistema
> top-10 Laravel (Sanctum, Horizon, Telescope, spatie/*, Filament,
> Socialite, Livewire, Debugbar, Excel, Dompdf) sem `lerd php:ext add`.

```
oci8        memcached  amqp        redis      imagick    mongodb
igbinary    pcov       xdebug      spx        opcache
curl        gd         intl        zip        pdo_mysql  pdo_pgsql
mysqli      soap       xsl         ldap       pcntl      exif
bcmath      mbstring   gmp         bz2        sysv*      sockets
calendar    dba        shmop       (e tudo que o php:X.Y-fpm-alpine já traz)
```

⚠️ **PHP 5.6 (legacy estendida)** vem sem `memcached`/`amqp`/`pcov`/`spx`
(extensões PECL atuais não compilam em 5.6). Tem oci8 2.0.12 + xdebug
2.5.5 + redis 4.3 + imagick + mongodb 1.7. Para apps Laravel 5.x legados
que precisam falar com Oracle.

---

## Instalação

### Pré-requisitos

- Linux (Arch, Fedora/Nobara, Debian/Ubuntu, openSUSE) ou macOS
- **Podman 4+** (rootless, sem Docker)
- `git`, `curl` ou `wget`

### Instalar

```bash
curl -fsSL https://raw.githubusercontent.com/gabriel-sousa99/lerd/main/install.sh | bash
```

ou via `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/gabriel-sousa99/lerd/main/install.sh | bash
```

O script:

1. Verifica `podman`, `git`, `mkcert` (instala via gerenciador do seu distro se faltar)
2. Baixa o binário pré-compilado para `$HOME/.local/bin/lerd`
3. Pergunta se quer ativar DNS gerenciado pelo lerd
   - **Padrão neste fork: NÃO** → seus sites ficam em `http://meusite.localhost/`
   - Sim → instala `lerd-dns` (dnsmasq containerizado, sites em `meusite.test`)

> [!TIP]
> Sem sudo, sem mexer no resolver do sistema. `*.localhost` resolve para
> loopback em qualquer SO moderno (RFC 6761).

### Verificar instalação

```bash
lerd about
# Deve exibir: "Lerd Oracle Edition" e versão 1.21.2-oracle.N
```

---

## Instalando no WSL2 (Ubuntu/Debian)

O lerd roda no WSL2, mas algumas premissas que ele assume em Linux nativo
(systemd ativo, NetworkManager/resolved gerenciando DNS, browser local)
não valem por padrão no WSL. Esta seção descreve a configuração mínima
para que tudo funcione.

> [!IMPORTANT]
> **Distros suportadas:** Ubuntu 22.04+ ou Debian 12+ rodando em WSL ≥
> 0.67.6 (que trouxe suporte oficial a systemd). Versões mais antigas
> precisam de workarounds tipo `genie`/`subsystemctl` que não são
> testados.

### 1. Habilitar systemd no WSL2

Toda a arquitetura do lerd (Quadlet, watcher, UI, FPM) roda como user
units do systemd. Sem systemd, **nada inicia**.

Dentro da distro WSL, crie/edite `/etc/wsl.conf`:

```ini
[boot]
systemd=true

[user]
default=seu-usuario
```

Depois, no **PowerShell do Windows**:

```powershell
wsl --shutdown
```

Reabra o terminal WSL e confirme:

```bash
ps -p 1 -o comm=
# deve imprimir: systemd

systemctl --user status
# deve responder sem erro
```

### 2. Configurar `networkingMode=mirrored` (recomendado)

Sem isso, `http://meusite.localhost` só responde **dentro** do WSL — o
Chrome do Windows não acha. Com o modo espelhado, a interface de rede
do WSL2 reflete a do Windows e `localhost` funciona dos dois lados.

No Windows, edite/crie `%USERPROFILE%\.wslconfig`:

```ini
[wsl2]
networkingMode=mirrored
dnsTunneling=true
firewall=true
autoProxy=true
```

Depois `wsl --shutdown` de novo.

> [!NOTE]
> Requer WSL ≥ 2.0.0 e Windows 11 22H2+. Sem mirrored networking você
> ainda consegue acessar pelos IPs da VM (`ip addr show eth0`) ou usar
> o browser dentro do WSL com `wslview`/`xdg-open`.

### 3. Instalar pacotes base

```bash
sudo apt update
sudo apt install -y \
  podman uidmap fuse-overlayfs slirp4netns \
  curl git unzip libnss3-tools
```

- `podman` + `uidmap` + `slirp4netns` → rootless containers
- `fuse-overlayfs` → fallback se o overlayfs nativo do kernel WSL falhar
- `libnss3-tools` → fornece `certutil` para o `mkcert`

> [!TIP]
> Não use `docker.io`/`docker-ce` no WSL2 com lerd — ele é construído
> exclusivamente para Podman rootless + Quadlet.

### 4. Habilitar linger

Garante que os containers continuam rodando depois que você fecha o
terminal:

```bash
sudo loginctl enable-linger $USER
```

Faça **logout e login de novo** (ou `wsl --shutdown` + reabrir) para a
sessão pegar a mudança.

### 5. Rodar o instalador em modo `.localhost`

```bash
curl -fsSL https://raw.githubusercontent.com/gabriel-sousa99/lerd/main/install.sh | bash
```

Quando perguntar **"Enable lerd-managed DNS for *.test domains?"** responda
**N** (não). O fork já vem com `*.localhost` como padrão, que resolve
para loopback automaticamente (RFC 6761) sem precisar de
`systemd-resolved`/`NetworkManager` — coisas que normalmente não estão
rodando no WSL2.

> [!WARNING]
> **Não escolha o modo `.test`** no WSL2. O setup de DNS em
> `internal/dns/setup.go` precisa de NetworkManager ou systemd-resolved
> ativos para instalar dispatcher/drop-in, e nenhum dos dois roda por
> padrão no WSL2 Ubuntu/Debian. Vai falhar feio.

### 6. Mantenha projetos em `$HOME` (NÃO em `/mnt/c/...`)

Esta é a otimização **mais importante** no WSL2. Bind mounts de
`/mnt/c/...` para dentro de containers passam pelo 9P (filesystem
remoto), que é **ordens de magnitude mais lento** que `ext4` nativo do
WSL2.

```bash
# ❌ EVITAR (Composer/npm install vão demorar 10x)
cd /mnt/c/Users/seu-user/projetos/meu-projeto
lerd init

# ✅ FAZER
mkdir -p ~/projetos
cd ~/projetos
git clone git@github.com:org/meu-projeto.git
cd meu-projeto
lerd init
```

Para editar com o VS Code do Windows, use a extensão **Remote – WSL** e
abra a pasta direto do WSL (`code .` dentro do WSL abre o VS Code
Windows apontado para o ext4).

### 7. HTTPS / mkcert no WSL2

O `mkcert -install` que o lerd dispara instala a CA na trust store do
**WSL**, não do Windows. Resultado:

- Browser dentro do WSL (Firefox/Chromium instalado via apt): confia ✓
- Browser do Windows (Chrome/Edge/Firefox): **não confia** ✗

Opções:

- **Dev local:** use HTTP (`http://meusite.localhost`). Suficiente pra
  90% dos casos.
- **HTTPS sério:** exporte a CA do mkcert e importe no Windows
  manualmente:
  ```bash
  cp "$(mkcert -CAROOT)/rootCA.pem" /mnt/c/Users/$USER/Desktop/lerd-rootCA.crt
  # depois no Windows: duplo-clique → Instalar → "Autoridades de
  # certificação raiz confiáveis"
  ```

### 8. Tray icon não funciona

O `lerd-tray` precisa de uma bandeja gráfica (`StatusNotifierItem` /
`AppIndicator`), que o WSL2 não fornece nativamente. Sem prejuízo
funcional — você pilota tudo via CLI ou pelo dashboard web em
`http://lerd.localhost`.

Se quiser desativar de vez para não ver erros de log:

```bash
systemctl --user mask lerd-tray.service
```

### 9. Auto-start do Oracle XE de teste

Se for usar o preset `oracle-xe` (Oracle XE 21c local), confira que a
imagem `gvenzl/oracle-xe:21-slim-faststart` baixa por completo (~2.5 GB)
**antes** de tentar `lerd service start oracle-xe`. WSL2 tem disco
dinâmico — se acabar o espaço no `.vhdx`, o serviço falha silencioso.
Verifique:

```bash
df -h ~/.local/share/containers
```

### Checklist final

Antes do `lerd init` do primeiro projeto, valide tudo:

```bash
# systemd como PID 1
ps -p 1 -o comm=                                  # systemd

# user session de pé
systemctl --user is-active default.target         # active

# linger
loginctl show-user $USER --property=Linger        # Linger=yes

# podman rootless
podman info --format '{{.Host.Security.Rootless}}' # true

# overlayfs ok (ou fuse-overlayfs caindo no fallback)
podman info --format '{{.Store.GraphDriverName}}'  # overlay

# UI do lerd subiu
systemctl --user is-active lerd-ui                # active

# resolução do .localhost
getent hosts teste.localhost                      # ::1 / 127.0.0.1
```

Se tudo verde, `cd ~/projetos/meu-projeto && lerd init` deve funcionar
exatamente como em Linux nativo.

### Troubleshooting WSL2

| Sintoma                                          | Solução                                                          |
|--------------------------------------------------|------------------------------------------------------------------|
| `System has not been booted with systemd as init` | `[boot] systemd=true` em `/etc/wsl.conf` + `wsl --shutdown`     |
| `Failed to connect to bus` em `systemctl --user`  | falta `loginctl enable-linger` ou faltou relogar                |
| `http://...localhost` não abre no Chrome Windows  | falta `networkingMode=mirrored` no `.wslconfig`                 |
| Composer / npm install lentíssimo                 | projeto está em `/mnt/c/...` — mova para `~/projetos/`          |
| `podman build` falha com overlay                  | `sudo apt install fuse-overlayfs` + `podman system reset`       |
| `lerd doctor` reclama de DNS                      | você optou pelo modo `.test` — reinstale e escolha `.localhost` |
| Erro `cannot allocate memory` em builds grandes   | aumente RAM da VM: `[wsl2] memory=8GB` no `.wslconfig`          |

---

## Primeiro uso

Dentro de qualquer projeto PHP:

```bash
cd ~/meu-projeto-laravel
lerd init     # wizard interativo (PHP, DB, HTTPS, serviços, workers)
lerd link     # registra o site (já chamado pelo init)
lerd open     # abre no navegador em http://meu-projeto-laravel.localhost
```

O wizard pergunta:

- **Versão do PHP** (7.4, 8.0, 8.1, 8.2, 8.3, 8.4, 8.5)
- **Versão do Node** (22, 24)
- **HTTPS?** (S/N — cria certificado via mkcert)
- **Database**:
  - SQLite (sem container)
  - MySQL (lerd-mysql)
  - PostgreSQL (lerd-postgres)
  - **Oracle (externo, oci8 já embutido)** ← novo
- **Serviços extras** (Redis, Meilisearch, RustFS/S3, Mailpit, Gotenberg, etc.)

Ao escolher **Oracle**, abre sub-form pedindo:

- Host (ex: `oracle.example.com`)
- Porta (padrão `1521`)
- Service name / SID (ex: `XEPDB1`, `ORCLPDB1`)
- Usuário
- Senha

Os valores vão para `.lerd.yaml` em um bloco `oracle:` e o `.env` recebe
`DB_CONNECTION=oracle` + `DB_HOST` + `DB_PORT` + `DB_DATABASE` (= service
name) + `DB_USERNAME` + `DB_PASSWORD`. Lerd não tenta subir container
Oracle (é DB externo).

---

## Trabalhando com Oracle

### Cliente Laravel: instalar `yajra/laravel-oci8`

```bash
lerd composer require yajra/laravel-oci8
lerd php artisan vendor:publish --provider="Yajra\Oci8\Oci8ServiceProvider" --tag=oracle --force
```

### Confirmar que `oci8` carregou

```bash
lerd php -r 'var_dump(extension_loaded("oci8"));'
# bool(true)
lerd php --ri oci8 | head -6
#   OCI8 Version => 3.4.1
#   Oracle Run-time Client Library Version => 21.18.0.0.0
```

### Subir um Oracle de teste local (Oracle XE 21)

Para validação sem depender do servidor corporativo:

```bash
podman run -d --name lerd-oracle-test \
  -p 1521:1521 \
  -e ORACLE_PASSWORD=lerd \
  -e ORACLE_DATABASE=LERDPDB \
  -e APP_USER=lerd_app \
  -e APP_USER_PASSWORD=lerd \
  docker.io/gvenzl/oracle-xe:21-slim-faststart
```

Esperar ~30s (`podman logs -f lerd-oracle-test`) até "DATABASE IS READY".
Depois configure o `.env` do seu projeto:

```dotenv
DB_CONNECTION=oracle
DB_HOST=127.0.0.1
DB_PORT=1521
DB_DATABASE=LERDPDB
DB_USERNAME=lerd_app
DB_PASSWORD=lerd
DB_CHARSET=AL32UTF8
```

E rode:

```bash
lerd php artisan migrate
```

### Charset / NLS_LANG

O `.lerd.yaml` aceita um campo opcional `charset:` no bloco oracle.
Quando definido, `lerd env` escreve `DB_CHARSET` + `NLS_LANG`:

```yaml
oracle:
  host: oracle.example.com
  port: 1521
  service_name: PRODPDB
  username: app_user
  password: ${ORACLE_PASSWORD}   # use placeholder e set no shell
  charset: AL32UTF8              # ou WE8MSWIN1252, WE8ISO8859P15
```

---

## Serviços (start, stop, status)

Lerd gerencia serviços via systemd user units. Comandos universais:

```bash
lerd service start <nome>      # ex: lerd service start mysql
lerd service stop <nome>
lerd service restart <nome>
lerd service status            # lista todos com estado
lerd service list              # nomes de presets disponíveis
```

Presets padrão: `mysql`, `mariadb`, `postgres`, `mongo`, `redis`,
`meilisearch`, `elasticsearch`, `rustfs`, `mailpit`, `gotenberg`,
`memcached`, `mongo-express`, `pgadmin`, `phpmyadmin`, etc.

### Parar tudo

```bash
lerd quit          # para containers + UI + watcher + tray
```

### Pausar um único site (mantém serviços)

```bash
cd ~/meu-projeto
lerd pause         # nginx vhost vira landing page
lerd unpause       # retoma
```

---

## Rebuild de imagens PHP

Quando você muda a Containerfile ou quer forçar reconstrução:

```bash
lerd php:rebuild              # rebuilda todas versões instaladas
lerd php:rebuild 8.4          # só a 8.4
lerd php:rebuild --local      # constrói tudo do zero (sem pull do ghcr)
```

> [!NOTE]
> Como o template Containerfile deste fork difere do upstream, o hash
> SHA-256 dos pulls do `ghcr.io/geodro/lerd-php<X>-fpm-base` **não bate**
> e o lerd cai automaticamente no build local — é o comportamento correto
> e garante que suas customizações (Instant Client, oci8, memcached, amqp)
> fiquem na imagem.

### Adicionar uma extensão por cima

Para uma única extensão extra (ex: `pdo_dblib`), continue usando:

```bash
lerd php:ext add pdo_dblib 8.4 --apk-deps "freetds-dev"
lerd php:ext list
lerd php:ext remove pdo_dblib 8.4
```

---

## Atualização

```bash
lerd update                # busca último release em gabriel-sousa99/lerd
lerd update --beta         # se houver pre-release marcada
lerd update --rollback     # volta para a versão anterior (backup automático)
```

O `lerd update` deste fork:

1. Consulta `https://github.com/gabriel-sousa99/lerd/releases/latest`
2. Compara versão atual com `1.21.2-oracle.N`
3. Faz download + substituição atômica do binário em `~/.local/bin/lerd`
4. Roda `lerd install --from-update` para reaplicar quadlets/DNS/sysctl
5. Se a Containerfile mudou: roda `lerd php:rebuild`

> [!WARNING]
> **Não** use `lerd-installer --update` apontado para o upstream — ele
> sobrescreve o binário com a versão sem suporte Oracle.

---

## Desinstalação

```bash
lerd-installer --uninstall
```

O script remove:

- `~/.local/bin/lerd`, `~/.local/bin/lerd-tray`, `~/.local/bin/lerd-installer`
- Unidades systemd user em `~/.config/systemd/user/lerd-*`
- Diretórios `~/.config/lerd/`, `~/.cache/lerd/`, `~/.local/share/lerd/`

> [!CAUTION]
> Os dados dos serviços (MySQL, Postgres, MinIO/RustFS) ficam em
> `~/.local/share/lerd/data/` e são apagados pelo `--uninstall`. Faça
> backup antes (`lerd db:export <site>` por exemplo).

Para limpar imagens podman também:

```bash
podman ps -a --filter "name=lerd-" -q | xargs podman rm -f
podman images --filter "reference=lerd-*" -q | xargs podman rmi -f
podman images --filter "reference=ghcr.io/geodro/lerd-*" -q | xargs podman rmi -f
```

---

## Diagnóstico

```bash
lerd doctor               # verifica DNS, podman, certs, services, versão
lerd bug-report           # gera arquivo com tudo necessário pra abrir issue
lerd dns:check            # diagnóstico em camadas do resolver
lerd logs <site>          # logs do FPM ou nginx do projeto atual
lerd logs <service>       # ex: lerd logs mysql
```

---

## Recursos exclusivos do dashboard

Algumas das adições da fork que vivem no painel web (`http://lerd.localhost`):

### Editor de `.env` (sites + serviços)

- **Site** → aba **Env**: textarea editável com Save/Descartar/Ctrl+S, dirty badge, beforeunload prompt. Backend cria `.env.before_lerd` automático na primeira edição (restaurável via `lerd env:restore`).
- **Serviço** → aba **Env**: editor key=value pro bloco `Environment=` do quadlet. Source badge mostra "preset" vs "override do usuário". + adicionar variável / botão remover por linha. Save escreve em `~/.config/lerd/services/<name>.yaml` — precisa Restart do serviço pra aplicar (avisado na UI).

### Gerenciamento de extensões PHP

`System → PHP X.Y → Extensões customizadas`: chips de exemplo rápido (`imap`, `swoole`, `ssh2`, `apcu`, `event`, `pspell`, `tidy`, `pdo_dblib`) que pré-preenchem nome + `apk-deps`, ou form livre. Build com spinner enquanto reconstrói. Equivalente a `lerd php:ext add <ext> X.Y --apk-deps "..."`.

### Instalador de versão PHP com logs ao vivo

`System → Instalar versão…`: lista versões disponíveis ainda não instaladas (5.6 / 7.4 / 8.0 / 8.1 / 8.2 / 8.3 / 8.4 / 8.5) com 1 clique de instalação. **Logs SSE ao vivo** durante o build (apk add / pecl install / COPY layers / etc.) com auto-scroll + "Copiar log". **beforeunload warning** evita fechar a aba no meio do build.

### Comandos artisan customizados (Laravel)

Dropdown **Commands** em cada site Laravel agora inclui **comandos descobertos automaticamente** em `app/Console/Commands/*.php` — extraídos via regex do `$signature` + `$description` (sem rodar PHP). Ícone ▶ pra distinguir dos defaults do framework.

⛔ **Filtro de destrutivos** em duas camadas:

1. List filter: `GET /api/sites/{d}/commands` nunca retorna `migrate:fresh`, `db:wipe`, `schema:drop`, `doctrine:fixtures:load`, `queue:flush`, `DROP TABLE`, `rm -rf /`, etc.
2. Runtime block: `handleCommandRun` retorna HTTP 403 mesmo se o comando passar pela lista.

Pra rodar mesmo assim → sempre via CLI: `lerd php artisan migrate:fresh --force`.

### Debug & Troubleshoot

`System → Debug & Troubleshoot`: botões pra rodar diagnósticos contra a instalação atual (`lerd doctor`, `dns:check`, `podman ps -a`, últimos logs) + grid de 9 cards pros guias do `docs/debug/*.md`. Botão "Copiar relatório" gera um bundle pra colar em issue.

### Botão "Abrir no editor" ao lado do terminal

Em cada site: ícone `</>` ao lado do terminal abre o projeto no editor GUI. Sonda: `$EDITOR_GUI` → `code` / `code-insiders` / `codium` / `cursor` → JetBrains (`phpstorm` / `webstorm` / `idea` / `goland`) → `subl` / `zed` / `nova`. macOS: `open -a "Visual Studio Code"` etc.

### Serviços novos (presets)

- **`oracle-xe`** (gvenzl/oracle-xe:21-slim-faststart) — Oracle XE 21c local pra dev. Cria automaticamente `LERDPDB` + usuário `lerd_app/lerd`. `userns: keep-id:uid=54321,gid=54321` + `chown_data` pra funcionar rootless. NLS_LANG pt-BR.
- **`typesense`** (typesense/typesense:28.0) — search engine open-source, alternativa Meilisearch/Algolia. Configura `SCOUT_DRIVER=typesense` no `.env`.
- **`typesense-dashboard`** (bfritscher/typesense-dashboard) — companion web pra typesense, segue o padrão do `pgadmin/postgres`.

---

## Debug e troubleshooting

Quando algo quebra, comece por:

```bash
lerd doctor                    # bateria de checagens (DNS, podman, certs, services)
lerd dns:check                 # diagnóstico em camadas do resolver
lerd bug-report                # gera arquivo .tar.gz com tudo necessário pra abrir issue
```

Guias por tópico em [`docs/DEBUG.md`](docs/DEBUG.md):

| Sintoma                                    | Onde olhar                                                             |
|--------------------------------------------|------------------------------------------------------------------------|
| Site retorna `502 Bad Gateway`             | [`debug/nginx.md`](docs/debug/nginx.md) + [`debug/php-fpm.md`](docs/debug/php-fpm.md) |
| `.localhost` ou `.test` não resolve        | [`debug/dns.md`](docs/debug/dns.md)                                    |
| Quadlet/systemd falha ao subir             | [`debug/podman.md`](docs/debug/podman.md)                              |
| `ORA-12541` / `ORA-12154` / `ORA-01017`    | [`debug/oracle.md`](docs/debug/oracle.md)                              |
| `lerd update` quebrou                      | [`debug/updates.md`](docs/debug/updates.md)                            |
| Worker em loop / fila parou                | [`debug/workers.md`](docs/debug/workers.md)                            |
| Conflito de porta em MySQL/Postgres        | [`debug/services.md`](docs/debug/services.md)                          |

Também acessível direto pelo dashboard em **System → Debug & Troubleshoot**, com botões pra rodar os diagnósticos e copiar o relatório.

## Lista de comandos úteis

| Comando                                | O que faz                                                  |
|----------------------------------------|------------------------------------------------------------|
| `lerd init`                            | Wizard interativo cria `.lerd.yaml`                        |
| `lerd link`                            | Registra o diretório como site                             |
| `lerd unlink`                          | Remove o site (sem deletar arquivos)                       |
| `lerd open`                            | Abre o site no navegador                                   |
| `lerd dashboard`                       | Abre o painel web (Cmd+K, live widgets)                    |
| `lerd tui`                             | Painel terminal estilo btop                                |
| `lerd php <args>`                      | Roda php no container do projeto (ex: `lerd php artisan tinker`) |
| `lerd composer <args>`                 | Roda composer com binários `composer-global` no PATH       |
| `lerd npm <args>` / `lerd npx <args>`  | Usa Node do projeto via fnm                                |
| `lerd db:shell`                        | Abre shell do DB do projeto                                |
| `lerd db:export` / `lerd db:import`    | Backup / restore                                           |
| `lerd db:isolate`                      | DB próprio para o worktree atual (clone do parent ou vazio)|
| `lerd horizon:start` / `:stop`         | Laravel Horizon como serviço systemd                       |
| `lerd queue:start` / `:stop`           | Worker de filas                                            |
| `lerd schedule:start` / `:stop`        | `php artisan schedule:work`                                |
| `lerd reverb:start` / `:stop`          | WebSocket server (Laravel Reverb)                          |
| `lerd lan`                             | Expõe sites para o LAN                                     |
| `lerd remote-control`                  | Liga/desliga acesso ao dashboard via LAN                   |
| `lerd mcp:enable-global`               | Registra MCP server para Claude / IDE / agentes            |
| `lerd php:install <ver>`               | **(fork)** Provisiona nova versão (5.6 → 8.5) — build + quadlet + start |
| `lerd php:rebuild [ver]`               | Reconstrói image FPM (após mudar Containerfile)            |
| `lerd php:ext add <ext> [ver]`         | Instala extensão extra via PECL + apk-deps                 |
| `lerd php:ini <ver>`                   | Edita `~/.local/share/lerd/php/<ver>/98-user.ini` (com validação) |
| `lerd service preset <name>`           | Instala um service preset (ex: `oracle-xe`, `typesense`)   |
| `lerd service start/stop/restart`      | Controle do serviço                                        |

---

## Compilando do código

```bash
git clone https://github.com/gabriel-sousa99/lerd.git
cd lerd
make build              # binário em build/lerd
make install            # copia para ~/.local/bin/
make test               # go test ./...
```

Requisitos: Go 1.25+, Node 22+, npm 10+.

---

## Créditos

- **Lerd** original — [George Dumitrescu](https://github.com/geodro) ([geodro/lerd](https://github.com/geodro/lerd))
- **Suporte Oracle** (este fork) — [Gabriel Sousa](https://github.com/gabriel-sousa99)

Licença: MIT (herdada do upstream — ver [`LICENSE`](LICENSE)).
