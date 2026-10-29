"""Bump the host tool pins in tools.yaml to their newest upstream releases."""
import functools
import hashlib
import json
import re
import sys
import time
import urllib.request

FILE = 'internal/tools/tools.yaml'
GITHUB = {'fnm': 'Schniz/fnm', 'mise': 'jdx/mise'}
STATIC_PHP = 'https://dl.static-php.dev/static-php-cli/common/?format=json'
# Edited line by line rather than through a YAML library, so the comments and
# quoting a bump PR leaves behind are exactly the ones a human wrote.
LINE = re.compile(r'^( *)([^#\s][^:]*):(?: (.*))?$')


def parse(lines):
    """Map each (tool, key[, platform]) under tools: to its line number."""
    index, path = {}, []
    for i, line in enumerate(lines):
        m = LINE.match(line)
        if not m:
            continue
        depth = len(m.group(1)) // 2
        path = path[:depth] + [m.group(2)]
        if path[0] == 'tools' and depth >= 1:
            index[tuple(path[1:])] = i
    return index


def get(lines, index, *key):
    return LINE.match(lines[index[key]]).group(3).strip('"')


def put(lines, index, value, *key):
    m = LINE.match(lines[index[key]])
    if m.group(3).startswith('"'):
        value = f'"{value}"'
    lines[index[key]] = f'{m.group(1)}{m.group(2)}: {value}'


def newest_php_patch(listing, minor):
    """The newest patch of a PHP minor that static-php.dev ships for all four platforms."""
    pattern = re.compile(r'^php-(%s\.\d+)-cli-(?:linux|macos)-(?:x86_64|aarch64)\.tar\.gz$' % re.escape(minor))
    builds = {}
    for entry in listing:
        m = pattern.match(entry['name'])
        if m:
            builds[m.group(1)] = builds.get(m.group(1), 0) + 1
    complete = [v for v, n in builds.items() if n == 4]
    return max(complete, key=lambda v: [int(p) for p in v.split('.')], default=None)


def upstream(tool, get_json):
    if tool == 'composer':
        return get_json('https://getcomposer.org/versions')['stable'][0]['version']
    if tool in GITHUB:
        return get_json(f'https://api.github.com/repos/{GITHUB[tool]}/releases/latest')['tag_name']
    if tool.startswith('php-host-'):
        return newest_php_patch(get_json(STATIC_PHP), tool.removeprefix('php-host-'))
    return None


def asset_url(lines, index, tool, version, platform):
    asset = get(lines, index, tool, 'assets', platform).replace('{version}', version)
    return get(lines, index, tool, 'url').replace('{version}', version).replace('{asset}', asset)


def bump(lines, tool, version, download):
    """Pin tool at version. Its digests and sizes are recomputed from the new
    assets, since every install checks a download against them."""
    index = parse(lines)
    if version == get(lines, index, tool, 'version'):
        return False
    put(lines, index, version, tool, 'version')
    if (tool, 'digests') not in index:
        return True
    for key in [k for k in index if k[:2] == (tool, 'assets') and len(k) == 3]:
        data = download(asset_url(lines, index, tool, version, key[2]))
        put(lines, index, hashlib.sha256(data).hexdigest(), tool, 'digests', key[2])
        if (tool, 'sizes') in index:
            put(lines, index, str(len(data)), tool, 'sizes', key[2])
    return True


def fetch(url, attempts=3):
    req = urllib.request.Request(url, headers={'User-Agent': 'lerd-tool-bump'})
    for attempt in range(1, attempts + 1):
        try:
            with urllib.request.urlopen(req, timeout=120) as resp:
                return resp.read()
        except OSError:
            if attempt == attempts:
                raise
            time.sleep(2 * attempt)


@functools.cache
def fetch_json(url):
    return json.loads(fetch(url))


def main():
    with open(FILE) as f:
        lines = f.read().split('\n')
    for tool in [k[0] for k in parse(lines) if len(k) == 1]:
        try:
            version = upstream(tool, fetch_json)
        except (OSError, KeyError, ValueError) as e:
            print(f'Skipping {tool}: upstream lookup failed ({e})')
            continue
        if not version:
            print(f'Skipping {tool}: no upstream release found')
            continue
        if bump(lines, tool, version, fetch):
            print(f'Bumped {tool} to {version}')
    with open(FILE, 'w') as f:
        f.write('\n'.join(lines))


if __name__ == '__main__':
    sys.exit(main())
