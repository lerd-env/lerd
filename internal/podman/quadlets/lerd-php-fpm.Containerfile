FROM docker.io/library/composer:latest AS composer-bin
FROM docker.io/library/php:{{.Version}}-fpm-alpine

RUN apk update && apk add --no-cache \
        autoconf \
        make \
        g++ \
        git \
        ghostscript \
        curl-dev \
        libzip-dev \
        libpng-dev \
        libjpeg-turbo-dev \
        freetype-dev \
        libwebp-dev \
        icu-dev \
        icu-data-full \
        oniguruma-dev \
        libxml2-dev \
        postgresql-dev \
        linux-headers \
        imagemagick-dev \
        imagemagick \
        gmp-dev \
        bzip2-dev \
        openldap-dev \
        sqlite-dev \
        libxslt-dev \
        mysql-client \
    && docker-php-ext-configure gd --with-freetype --with-jpeg --with-webp \
    && docker-php-ext-install -j$(nproc) \
        curl \
        pdo_mysql \
        pdo_pgsql \
        bcmath \
        mbstring \
        xml \
        zip \
        gd \
        intl \
        pcntl \
        exif \
        sockets \
        gmp \
        bz2 \
        calendar \
        dba \
        ldap \
        mysqli \
        soap \
        shmop \
        sysvmsg \
        sysvsem \
        sysvshm \
        xsl \
    && (docker-php-ext-enable opcache || true) \
    && { (pecl install redis && docker-php-ext-enable redis) \
         || (git clone --depth 1 https://github.com/phpredis/phpredis /tmp/phpredis \
             && cd /tmp/phpredis && phpize && ./configure && make -j$(nproc) && make install \
             && docker-php-ext-enable redis \
             && rm -rf /tmp/phpredis) \
         || true; } \
    && { (pecl install imagick && docker-php-ext-enable imagick) \
         || (git clone --depth 1 https://github.com/Imagick/imagick /tmp/imagick \
             && cd /tmp/imagick && phpize && ./configure && make -j$(nproc) && make install \
             && docker-php-ext-enable imagick \
             && rm -rf /tmp/imagick) \
         || true; } \
    && { (pecl install igbinary && docker-php-ext-enable igbinary) || true; } \
    && { (pecl install mongodb && docker-php-ext-enable mongodb) || true; } \
    && { (pecl install pcov && docker-php-ext-enable pcov) || true; } \
    && rm -rf /tmp/pear /var/cache/apk/*

# MariaDB client (mysql-client) connecting to lerd MySQL uses self-signed certs;
# disable SSL verification so CLI tools (mysqldump, schema loading) work out of the box.
RUN mkdir -p /etc/my.cnf.d && printf '[client]\nssl=0\n' > /etc/my.cnf.d/lerd-no-ssl.cnf

# Install Composer, Node.js, and FFmpeg (used by media libraries like spatie/media-library)
COPY --from=composer-bin /usr/bin/composer /usr/local/bin/composer
RUN apk add --no-cache nodejs npm ffmpeg

# Interactive shell for `lerd shell` and the TUI shell action. Ships zsh with
# a self-contained config (starship prompt, persistent history). Host shell
# config is intentionally NOT mounted: every developer's host config is
# different, and sourcing distro-specific paths or host-only binaries
# cascades into noisy errors. The in-container shell is its own environment.
RUN apk add --no-cache zsh starship fzf eza bat zoxide \
    && mkdir -p /etc/zsh /root/.zsh_state \
    && printf 'export EDITOR=vi\nexport PAGER=less\nexport HISTFILE=/root/.zsh_state/history\nexport HISTSIZE=10000\nexport SAVEHIST=10000\nsetopt INC_APPEND_HISTORY SHARE_HISTORY\nautoload -Uz compinit && compinit -u\nif command -v starship >/dev/null 2>&1; then\n  eval "$(starship init zsh)"\nfi\n' \
        > /etc/zsh/zshrc

# Override pool: run workers as root, log errors to stderr
RUN printf '[www]\nuser=root\ngroup=root\ncatch_workers_output=yes\nphp_flag[display_errors]=off\nphp_admin_value[error_log]=/proc/self/fd/2\nphp_admin_flag[log_errors]=on\n' > /usr/local/etc/php-fpm.d/zz-lerd.conf

# Xdebug always installed; mode controlled via mounted ini (mode=off by default).
# Legacy PHP needs an older line: xdebug 3.2+ requires PHP 8.0+, 3.4+ requires 8.1+.
RUN PHPVER="$(php -r 'echo PHP_MAJOR_VERSION,".",PHP_MINOR_VERSION;')" \
    && case "$PHPVER" in \
        7.4) XDEBUG_PKG="xdebug-3.1.6" ;; \
        8.0) XDEBUG_PKG="xdebug-3.3.2" ;; \
        *)   XDEBUG_PKG="xdebug" ;; \
    esac \
    && pecl install "$XDEBUG_PKG" && docker-php-ext-enable xdebug \
    && rm -rf /tmp/pear /var/cache/apk/*

{{.CustomExtensions}}
{{.MkcertCA}}
