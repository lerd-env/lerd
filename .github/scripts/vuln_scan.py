"""Scan the published PHP base images and pick the ones a rebuild would fix."""
import hashlib
import json
import os
import subprocess
import sys
import urllib.error
import urllib.request

import upstream_php

# 7.4 and 8.0 sit on end-of-life Alpine branches, so a rebuild cannot help them.
VERSIONS = ('8.1', '8.2', '8.3', '8.4', '8.5', '8.6')
SEVERE = ('High', 'Critical')
RECIPE = 'internal/podman/quadlets/lerd-php-fpm.Containerfile'


def recipe_tag(recipe):
    for ph in ('{{.CustomExtensions}}', '{{.CustomExtensionsRuntime}}',
               '{{.CustomPackages}}', '{{.MkcertCA}}'):
        recipe = recipe.replace(ph, '')
    return hashlib.sha256(recipe.encode()).hexdigest()[:12]


def severe(report):
    return [m for m in report['matches'] if m['vulnerability']['severity'] in SEVERE]


def needs_rebuild(found):
    # Only Alpine packages are ours to fix; bundled npm, Rust and Go binaries
    # wait on their packagers, so they are listed, not rebuilt.
    return any(m['artifact']['type'] == 'apk' for m in found)


def rows(version, found):
    out = []
    for m in found:
        a, v = m['artifact'], m['vulnerability']
        out.append(f"| {version} | {a['type']} | {a['name']} {a['version']} | {v['id']} | {v['severity']} | {', '.join(v['fix']['versions'])} |")
    return out


def scan(image):
    out = subprocess.run(['docker', 'run', '--rm', os.environ['GRYPE'], f'registry:{image}',
                          '--only-fixed', '-o', 'json'], capture_output=True, text=True, check=True).stdout
    return json.loads(out)


def upgrades(image):
    """The packages apk upgrade would replace in image today."""
    out = subprocess.run(['docker', 'run', '--rm', image, 'sh', '-c', 'apk update -q >/dev/null && apk upgrade --simulate'],
                         capture_output=True, text=True, check=True).stdout
    return [line.strip() for line in out.splitlines() if 'Upgrading' in line]


def released(version):
    """Whether upstream publishes the php:<version>-fpm-alpine a rebuild builds FROM."""
    token = json.load(urllib.request.urlopen(
        'https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/php:pull',
        timeout=upstream_php.TIMEOUT))['token']
    try:
        upstream_php.fetch(f'https://registry-1.docker.io/v2/library/php/manifests/{version}-fpm-alpine', token)
    except urllib.error.HTTPError as e:
        if e.code == 404:
            return False
        raise
    return True


def main(scan_fn=scan, upgrades_fn=upgrades, released_fn=released):
    with open(RECIPE) as f:
        tag = recipe_tag(f.read())
    stale, table, failed = [], [], []
    for version in VERSIONS:
        image = f'ghcr.io/lerd-env/lerd-php{version.replace(".", "")}-fpm-base:{tag}-amd64'
        # One image missing, as while a new recipe still publishes, must not
        # drop the rebuilds the others need.
        try:
            found = severe(scan_fn(image))
            # A fix the advisory lists but Alpine does not serve yet would make
            # every rebuild republish the same packages, so only an image that
            # can upgrade something is rebuilt.
            rebuild = needs_rebuild(found) and bool(upgrades_fn(image))
            # A new minor has no upstream tag until its GA day, so a forced
            # rebuild of it could only fail.
            unreleased = rebuild and not released_fn(version)
        except (subprocess.CalledProcessError, ValueError, urllib.error.URLError, TimeoutError) as e:
            print(f'::error::PHP {version}: scanning {image} failed: {e}')
            failed.append(version)
            continue
        if unreleased:
            print(f'::notice::PHP {version}: flagged, but upstream has no {version}-fpm-alpine to rebuild from yet')
        elif rebuild:
            stale.append(version)
        elif needs_rebuild(found):
            print(f'::notice::PHP {version}: flagged, but Alpine has nothing newer to install yet')
        table += rows(version, found)
        print(f'PHP {version}: {len(found)} fixable high or critical')

    with open(os.environ['GITHUB_STEP_SUMMARY'], 'a') as s:
        s.write('| PHP | Type | Package | Vulnerability | Severity | Fixed in |\n|---|---|---|---|---|---|\n')
        s.write('\n'.join(table) + '\n')
    with open(os.environ['GITHUB_OUTPUT'], 'a') as out:
        out.write(f'stale={",".join(stale)}\n')
    return 1 if failed else 0


if __name__ == '__main__':
    sys.exit(main())
