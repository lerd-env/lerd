#!/usr/bin/env python3
# Generates the landing-page asciinema casts. Run from docs/:
#   python3 scripts/gen-casts.py
# Heights are sized to each cast's content so the terminal windows hug it.
import json, os

OUT = 'public/casts'
E = '\x1b'
C = dict(rst=E+'[0m', dim=E+'[38;5;245m', prompt=E+'[38;5;210m', cmd=E+'[38;5;255m',
         ok=E+'[38;5;42m', url=E+'[38;5;39m', vio=E+'[38;5;141m', amb=E+'[38;5;215m',
         pink=E+'[38;5;211m', bold=E+'[1m')


def col(name, s):
    return C[name] + s + C['rst']


class Cast:
    def __init__(self, w, h):
        self.w = w
        self.h = h
        self.t = 0.0
        self.ev = []

    def out(self, s):
        self.ev.append([round(self.t, 3), 'o', s])

    def wait(self, dt):
        self.t += dt

    def type(self, prompt, cmd):
        self.out(col('prompt', prompt))
        for ch in cmd:
            self.wait(0.045)
            self.out(col('cmd', ch))
        self.wait(0.35)
        self.out('\r\n')

    def line(self, s='', dt=0.45):
        self.wait(dt)
        self.out(s + '\r\n')

    def write(self, name):
        header = {'version': 2, 'width': self.w, 'height': self.h,
                  'theme': {'fg': '#f4f4f5', 'bg': '#0d0d0d',
                            'palette': '#0d0d0d:#ff5b50:#34d399:#fbbf24:#38bdf8:#a78bfa:#38bdf8:#c4c4cc'}}
        with open(os.path.join(OUT, name), 'w') as f:
            f.write(json.dumps(header) + '\n')
            for e in self.ev:
                f.write(json.dumps(e) + '\n')


os.makedirs(OUT, exist_ok=True)

# ---- hero (11 content lines) ----
c = Cast(74, 12)
c.type('~/code/acme $ ', 'lerd link')
c.line(col('dim', '→ detecting framework… ') + col('ok', 'Laravel 11'))
c.line(col('dim', '→ php ') + col('vio', '8.4') + col('dim', ' · node ') + col('vio', '22') + col('dim', ' · nginx vhost written'))
c.line(col('dim', '→ provisioning TLS via mkcert… ') + col('ok', '✓ trusted'))
c.line(col('ok', '✓ linked') + col('dim', ' in 1.8s'))
c.line('')
c.line('  ' + col('dim', 'Site') + '   ' + col('url', 'https://acme.test'))
c.line('  ' + col('dim', 'PHP') + '    8.4.3  ·  ' + col('dim', 'FPM') + ' ' + col('ok', 'running'))
c.line('  ' + col('dim', 'DB') + '     mysql  ·  ' + col('dim', 'cache') + ' redis')
c.line('')
c.type('~/code/acme $ ', 'lerd open')
c.wait(3.0)
c.write('hero.cast')

# ---- quick-start: install, then `lerd link` (which runs init/setup/TLS itself) ----
s1 = Cast(70, 7)
s1.type('$ ', 'curl -fsSL https://lerd.sh/install.sh | bash')
s1.line(col('dim', '→ installing podman, fpm, nginx (rootless)…'))
s1.line(col('ok', '✓ lerd ready'))
s1.wait(2.5)
s1.write('step-01.cast')

s2 = Cast(70, 7)
s2.type('~/code/acme $ ', 'lerd link')
s2.line(col('dim', '→ init · ') + col('ok', 'Laravel 12') + col('dim', ' · php ') + col('vio', '8.4') + col('dim', ' · node ') + col('vio', '22'))
s2.line(col('dim', '→ installing dependencies · running migrations'))
s2.line(col('dim', '→ starting ') + col('amb', 'queue · schedule · reverb') + col('dim', ' workers'))
s2.line(col('dim', '→ TLS via mkcert · ') + col('ok', '✓ trusted'))
s2.line(col('ok', '✓ live at ') + col('url', 'https://acme.test'))
s2.wait(3.0)
s2.write('step-02.cast')

# ---- mcp: a terminal Claude Code session driving lerd (16 content lines) ----
m = Cast(88, 17)
m.type('~/code $ ', 'claude')
m.line(col('dim', '╭─ Claude Code ───────────────── lerd-mcp connected ─╮'), 0.5)
m.line(col('dim', '╰────────────────────────────────────────────────────╯'), 0.2)
m.line('')
m.line(col('pink', '> ') + 'Spin up a fresh Laravel site called ' + col('bold', 'billing') + ' on PHP 8.4 with Redis.', 0.6)
m.line('')
m.line(col('amb', '● ') + 'On it — scaffolding and linking now.', 0.7)
m.line('  ' + col('dim', '⎿ ') + col('vio', 'site.link') + col('dim', ' (lerd) … ') + col('ok', 'done'), 0.9)
m.line('  Created ' + col('url', '~/code/billing') + ', detected Laravel 11, set PHP ' + col('bold', '8.4') + ', provisioned TLS.', 0.5)
m.line('  ' + col('dim', '⎿ ') + col('vio', 'service.start redis') + col('dim', ' (lerd) … ') + col('ok', 'done'), 0.9)
m.line('  Redis is up. Site live at ' + col('url', 'https://billing.test') + ' ' + col('ok', '✓'), 0.5)
m.line('')
m.line(col('pink', '> ') + 'Run the migrations and tail the queue.', 0.8)
m.line('')
m.line(col('amb', '● ') + col('dim', '⎿ ') + col('vio', 'exec.artisan migrate') + col('dim', ' (lerd) … ') + col('ok', 'done'), 0.9)
m.line('  Ran 18 migrations in 240ms. Tailing ' + col('vio', 'queue') + ' + ' + col('vio', 'dump') + ' for you.', 0.5)
m.wait(3.5)
m.write('mcp.cast')

print('casts:', sorted(os.listdir(OUT)))
