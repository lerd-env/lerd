# PHP-FPM — Debug

[← voltar pro índice](../DEBUG.md)

## Como funciona

Uma imagem por versão PHP: `lerd-php74-fpm:local`, `lerd-php83-fpm:local`, …, `lerd-php85-fpm:local`. Cada uma:

- Builder stage compila ~25 extensões + Xdebug + oci8 + Instant Client 21.18 + memcached + amqp
- Runtime stage copia só os `.so` + libs runtime (sem toolchain)
- O hash SHA-256 do template Containerfile vira a tag base no `ghcr.io` (cache pre-built)

```
   docker.io/library/php:X.Y-fpm-alpine
              │
              ▼
   [builder] apk add toolchain → docker-php-ext-install ... → pecl install ... → oci8 + Instant Client
              │
              ▼
   [runtime] apk add runtime libs → COPY .so files from builder → CA mkcert + zsh + composer
              │
              ▼
   lerd-php<short>-fpm:local
              │
              ▼ run as systemd user unit lerd-php<short>-fpm
   nginx → fastcgi → :9000 (php-fpm)
```

## Problemas comuns

### 🔴 `lerd php:rebuild` falhou em `pecl install <ext>`

🔍 Diagnóstico: o log do build mostra exatamente onde quebrou. Geralmente:

| Erro                                              | Causa                                                            |
|---------------------------------------------------|------------------------------------------------------------------|
| `Package "<ext>" does not have REST xml available`| Versão da extensão não suporta o PHP da imagem                   |
| `error: <header>.h: No such file or directory`    | Faltou pacote `-dev` em `apk-deps`                               |
| `make: *** [Makefile:N] Error 1`                  | Bug do source — tentar versão anterior do pacote pecl            |

🟢 Conserto:
```bash
# Pinar versão da extensão (no caso, ssh2 quebrando no 8.3):
lerd php:ext add ssh2-1.4.1 8.3 --apk-deps "libssh2-dev"
```

Ou removê-la se estiver bloqueando o build:
```bash
lerd php:ext remove <ext> 8.3
lerd php:rebuild 8.3
```

### 🔴 Extensão não aparece em `php -m` mesmo após instalar

🔍 Diagnóstico:
```bash
podman run --rm lerd-php85-fpm:local php -m | grep -i <ext>
podman run --rm lerd-php85-fpm:local sh -c 'ls /usr/local/lib/php/extensions/no-debug-non-zts-*/<ext>.so'
podman run --rm lerd-php85-fpm:local sh -c 'cat /usr/local/etc/php/conf.d/*<ext>*.ini'
```

🟢 Conserto: o `.so` existe mas o `docker-php-ext-enable` falhou — a verificação acontece automaticamente após `lerd php:ext add` (rolling back se falhar). Para casos manuais:
```bash
podman exec lerd-php85-fpm sh -c 'docker-php-ext-enable <ext> && kill -USR2 1'
```

### 🔴 `lerd php:ext add <ext>` instala mas FPM volta com 500

🔍 Diagnóstico:
```bash
podman logs lerd-php85-fpm 2>&1 | tail -30
# Procure por "PHP Fatal error" ou "Unable to load dynamic library"
```

🟢 Conserto: provavelmente a extensão precisa de uma lib runtime que foi instalada só no builder. Adicione no `apk-deps` ao instalar — o `lerd php:ext` já replica esses pacotes pro estágio runtime.

### 🔴 `lerd update` puxou nova versão mas a imagem PHP não rebuildou

🔍 Diagnóstico:
```bash
cat ~/.cache/lerd/fpm_image_hash 2>/dev/null    # hash da última build
cat <<'GO' | go run -                            # hash atual do template embarcado
... (não há jeito fácil sem o binário antigo)
GO
```

🟢 Conserto:
```bash
lerd php:rebuild                                # rebuilda todas
lerd php:rebuild 8.5 --local                    # rebuild 8.5 from scratch
```

### 🔴 Xdebug ativado mas IDE não recebe step

🔍 Diagnóstico:
```bash
cat ~/.local/share/lerd/php/8.5/99-xdebug.ini
# Deve ter: xdebug.client_host=host.containers.internal e xdebug.start_with_request=trigger
podman exec lerd-php85-fpm php -i | grep -i xdebug
```

🟢 Conserto:
```bash
# IDE precisa estar escutando na 9003 (não 9000, que é o FPM)
# E o request deve carregar cookie/header XDEBUG_TRIGGER (ou query ?XDEBUG_TRIGGER=1)
# Ou trocar pra start_with_request=yes (sempre liga, mais pesado)
lerd php:ini 8.5                                # editor com validação
```

### 🔴 Composer dentro do container não acha pacote global do host

⚠️ Esperado. O composer-global do container vive em `/root/.composer/`, separado do host. O `lerd composer` sincroniza o `bin/` global automaticamente — outros pacotes globais não.

🟢 Conserto:
```bash
lerd composer global require <pacote>           # instala dentro do container
# Pra usar:
lerd composer global show -i                    # lista o que tem
```

### 🔴 oci8 não carrega após `lerd php:install 8.3` numa máquina nova

Veja [`oracle.md`](oracle.md).

### 🔴 `composer update` falha com `cannot run ssh: No such file or directory`

⚠️ Container PHP **anterior** ao fork v1.21.2-oracle.10 não tinha `openssh-client`. O composer usa git pra clone via ssh e git precisa do binário `/usr/bin/ssh`.

🔍 Diagnóstico:
```bash
podman run --rm lerd-php85-fpm:local sh -c 'which ssh 2>&1 || echo NO_SSH'
```

🟢 Conserto: rebuild a imagem com a versão atual do fork (que tem o `openssh-client` no apk add do runtime stage):
```bash
lerd php:rebuild 8.5 --local      # ou a versão do projeto
```

### 🔴 `composer update` agora encontra ssh mas falha com `Permission denied (publickey)`

⚠️ ssh está no container, mas git procura chaves em `$HOME/.ssh` — que dentro do container é `/root/.ssh` (vazio), não `/home/gabriel/.ssh`.

🔍 Diagnóstico:
```bash
podman exec lerd-php85-fpm sh -c 'ls /root/.ssh 2>&1 || echo EMPTY'
cat ~/.config/containers/systemd/lerd-php85-fpm.container | grep -E "ssh|GIT_SSH"
```

🟢 Conserto: a partir de oracle.12 o quadlet monta `$HOME/.ssh:/root/.ssh:ro` automaticamente. Pra forçar a regeneração:
```bash
lerd php:install 8.5      # re-escreve o quadlet, mantém a imagem
systemctl --user restart lerd-php85-fpm
```

💡 Se sua chave tem passphrase, ssh-agent **não** é propagado pro container. Use chave sem passphrase **OU** exporte `SSH_AUTH_SOCK` e adicione `Volume=${SSH_AUTH_SOCK}:${SSH_AUTH_SOCK}` + `Environment=SSH_AUTH_SOCK=...` manualmente.

### 🔴 Xdebug emite "Could not connect to debugging client" em todo comando CLI

⚠️ Antes de oracle.9, o default era `xdebug.start_with_request=yes`, que tenta conectar em `host.containers.internal:9003` em CADA request. Sem IDE escutando = spam.

🔍 Diagnóstico:
```bash
cat ~/.local/share/lerd/php/8.5/99-xdebug.ini | grep start_with_request
```

🟢 Conserto: edite pra `=trigger` (default novo do fork) e restart o FPM:
```bash
sed -i 's/start_with_request=yes/start_with_request=trigger/' \
  ~/.local/share/lerd/php/8.5/99-xdebug.ini
systemctl --user restart lerd-php85-fpm
```

💡 Pra acionar o Xdebug agora você precisa de cookie/header `XDEBUG_TRIGGER=1`, ou query `?XDEBUG_TRIGGER=1`, ou a extension PhpStorm/VS Code Toolbox.

## 💡 Dicas

- `lerd php --ri oci8` (ou outra ext) mostra runtime info: versão, paths, configs ativos.
- `lerd php:list` mostra versões com a default marcada.
- O conteúdo de `~/.local/share/lerd/php/<v>/98-user.ini` sobrescreve o `php.ini` padrão — edite com `lerd php:ini <v>` (tem validador).
- Pra debugar segfault no FPM: `podman exec -it lerd-php85-fpm sh` → instala `gdb` → `apk add gdb` → `gdb php-fpm <pid>`.
