---
title: Moving from MAMP
description: 'Move your MAMP projects to Lerd on macOS or Linux: link each project, import the databases, and translate the .htaccess rules Apache was applying into nginx overrides.'
head:
  - - meta
    - name: keywords
      content: mamp alternative, mamp for linux, mamp linux, mamp to nginx, htaccess to nginx, mamp migration, mamp pro alternative, local php development
---

# Moving from MAMP

MAMP gives you Apache, MySQL and PHP in one folder on macOS and Windows. Lerd covers the same ground on macOS and Linux, with automatic `.test` domains, trusted HTTPS, a PHP version per project and shared databases, and it is free and open source.

The one real difference is the web server. MAMP serves through Apache, which reads `.htaccess` files. Lerd serves through nginx, which does not. Most projects need no changes at all, and this page shows how to carry over the ones that do.

## Every MAMP feature, and the Lerd equivalent

| What you used in MAMP | The same thing in Lerd |
|---|---|
| `localhost:8888/myapp` | `https://myapp.test`, one domain per project with no port and no hosts file edits |
| Projects inside `htdocs` | Projects anywhere, `lerd link` serves the directory you run it in |
| MAMP Pro hosts | Every linked project is a host, `lerd park ~/code` links a whole folder at once |
| MAMP Pro SSL | `lerd secure`, an mkcert certificate trusted by your system and browsers |
| PHP version per host | `lerd isolate 8.4`, or picked up from `composer.json` |
| MySQL on port 8889, user `root`, password `root` | `lerd service start mysql`, shared by every site, wired into `.env` by `lerd env` |
| phpMyAdmin | `lerd service start phpmyadmin`, or any database client on the published port |
| `.htaccess` rules | An [nginx override](/usage/nginx-overrides) per site, see below |
| `php.ini` from the MAMP menu | `lerd php:ini <version>`, or System → PHP in the web dashboard |

## Moving a project

**1. Export your databases.** In MAMP's phpMyAdmin, open each database and use the **Export** tab, or run MAMP's own `mysqldump` against port 8889 with user `root` and password `root`.

**2. Install Lerd and start it.**

```bash
curl -fsSL https://lerd.sh/install.sh | bash
lerd start
```

**3. Link the project.** Copy it out of `htdocs` to wherever you keep code, then:

```bash
cd ~/code/myapp
lerd link
```

Lerd detects the framework, sets the document root, writes the nginx vhost and registers `myapp.test`. A project whose entry point is not in the framework's usual folder can set [`public_dir`](/configuration#per-project-config-lerd-yaml) in `.lerd.yaml`.

**4. Restore the database.**

```bash
lerd service start mysql
lerd db:import myapp.sql
```

**5. Point the app at Lerd's services.** `lerd env` rewrites the database, cache and mail entries in `.env`, so the MAMP host, port 8889 and `root` password are replaced with Lerd's. On WordPress it writes the same values into `wp-config.php`.

## Do you need to translate your .htaccess?

Open the project's `.htaccess`. If all it contains is the usual "send everything that is not a file to `index.php`" block, you are done. That is the standard file Laravel, Symfony, WordPress, Drupal and most other frameworks ship, and Lerd's vhost already does the same thing:

```nginx
location / {
    try_files $uri $uri/ /index.php?$query_string;
}
```

Lerd also blocks access to `.htaccess` and `.htpasswd` files, just as Apache does.

Anything beyond that, redirects, blocked paths, headers, custom error pages, has to be written as nginx directives in the site's override. Open it from the sliders button in the site's address bar in the web dashboard, or with `lerd nginx edit`. Every save is checked with `nginx -t` before it goes live, so a typo is rejected rather than taking the site down.

## Translating common rules

| Apache `.htaccess` | nginx override |
|---|---|
| `Redirect 301 /old /new` | `location = /old { return 301 /new; }` |
| `RewriteRule ^blog/(.*)$ /news/$1 [R=301,L]` | `rewrite ^/blog/(.*)$ /news/$1 permanent;` |
| `RewriteRule ^shop/(.*)$ /store.php?item=$1 [L]` (internal, no redirect) | `rewrite ^/shop/(.*)$ /store.php?item=$1 last;` |
| Forcing HTTPS with `RewriteCond %{HTTPS} off` | Nothing, `lerd secure` already redirects HTTP to HTTPS |
| `<Files "config.php"> Require all denied </Files>` | `location = /config.php { deny all; }` |
| Denying a folder, `RedirectMatch 403 ^/private/` | `location ^~ /private/ { deny all; }` |
| `ErrorDocument 404 /404.php` | `error_page 404 /404.php;` |
| `Header set X-Frame-Options "SAMEORIGIN"` | `add_header X-Frame-Options "SAMEORIGIN" always;` |
| `ExpiresByType image/png "access plus 1 month"` | `location ~* \.png$ { expires 30d; }` |
| `Options -Indexes` | Nothing, nginx never lists directories unless told to |
| `php_value upload_max_filesize 64M` | Not nginx, raise it with `lerd php:ini` |

A few things to keep in mind while translating:

- **Paths start with a slash in nginx.** Apache strips the leading `/` before matching a `RewriteRule` in `.htaccess`, nginx does not, so `^blog/` becomes `^/blog/`.
- **`[L]` becomes `last`, `[R=301]` becomes `permanent`, `[R=302]` becomes `redirect`.**
- **Don't redefine `location /`.** The vhost already has one, and nginx refuses a duplicate. Use a more specific `location`, or a `rewrite` at the top level of the override, which runs before nginx picks a location.
- **`RewriteCond` has no direct equivalent.** Most conditions become an `if` on a variable, for example `if ($http_user_agent ~* badbot) { return 403; }`, or a `map`. Keep these small, nginx's `if` is not a general purpose branch.
- **Rules from a `.htaccess` in a subfolder** apply only under that folder in Apache. Wrap them in a `location ^~ /subfolder/ { ... }` block.

## WordPress

WordPress's own `# BEGIN WordPress` block is the front controller rule above, so permalinks work as soon as the site is linked. Two things are worth checking:

- **Anything outside that block.** Redirects you added by hand or through a plugin like Redirection's Apache mode, and hardening rules from security plugins, sit above or below it. Translate them with the table above.
- **Plugins that write `.htaccess` for you.** Caching plugins (W3 Total Cache, WP Super Cache, WP Rocket) and security plugins (Wordfence, iThemes / Solid Security, All In One WP Security) keep writing those rules, and nginx keeps ignoring them. The site still works, but the rule the plugin thinks is active is not. Most of these plugins have an nginx mode or print the nginx equivalent in their settings, paste that into the override instead.

See the [WordPress walkthrough](/getting-started/wordpress) for the rest of a WordPress setup on Lerd.

## Rules that do not translate

Some `.htaccess` features have no clean nginx counterpart. Password protection with `AuthType Basic` needs a password file the nginx container can read, and per-directory `php_flag` settings have no per-folder equivalent in PHP-FPM. If a project leans on these, the simpler route is usually to move the rule into the application itself.

## Next steps

- [Nginx overrides](/usage/nginx-overrides), the full reference for the per-site editor
- [Quick start](/getting-started/quick-start), a project served in two commands
- [Full comparison](/getting-started/comparison) against Laravel Herd, Laragon, DDEV and Lando
