"""Compare each published PHP base image with the upstream image it is built on.

Our image is FROM upstream, so it carries upstream's layers as a prefix; once
upstream republishes, that prefix stops matching and the version is rebuilt.
No saved state: the published base image is the record of what it was built
from. Writes changed=<versions> to GITHUB_OUTPUT for the rebuild step.
"""
import hashlib
import json
import os
import urllib.error
import urllib.request

# A prerelease is watched on its -rc tag (see config.UpstreamPHPTag);
# swap it for the plain one on release day.
VERSIONS = {'8.1': '8.1', '8.2': '8.2', '8.3': '8.3', '8.4': '8.4', '8.5': '8.5', '8.6': '8.6'}
ARCHES = ('amd64', 'arm64')
ACCEPT = ','.join('application/vnd.' + t for t in (
    'oci.image.index.v1+json', 'docker.distribution.manifest.list.v2+json',
    'oci.image.manifest.v1+json', 'docker.distribution.manifest.v2+json'))
CONTAINERFILE = 'internal/podman/quadlets/lerd-php-fpm.Containerfile'


def recipe_tag(recipe):
    """The base image tag the build computes from the Containerfile."""
    for ph in ('{{.CustomExtensions}}', '{{.CustomExtensionsRuntime}}',
               '{{.CustomPackages}}', '{{.MkcertCA}}'):
        recipe = recipe.replace(ph, '')
    return hashlib.sha256(recipe.encode()).hexdigest()[:12]


def fetch(url, token, accept=ACCEPT):
    req = urllib.request.Request(url, headers={'Authorization': f'Bearer {token}', 'Accept': accept})
    return json.load(urllib.request.urlopen(req))


def diff_ids(host, token_url, repo, ref, arch):
    token = json.load(urllib.request.urlopen(token_url))['token']
    m = fetch(f'https://{host}/v2/{repo}/manifests/{ref}', token)
    if 'manifests' in m:
        digest = next(x['digest'] for x in m['manifests']
                      if x['platform']['architecture'] == arch and x['platform']['os'] == 'linux')
        m = fetch(f'https://{host}/v2/{repo}/manifests/{digest}', token)
    return fetch(f'https://{host}/v2/{repo}/blobs/{m["config"]["digest"]}', token, '*/*')['rootfs']['diff_ids']


def changed_versions(versions, arches, upstream_ids, our_ids, log=print):
    """Versions whose base is not built on the current upstream image.

    upstream_ids(version, upstream_tag, arch) and our_ids(version, arch) return
    layer ids or raise urllib.error.HTTPError.
    """
    changed = []
    for version, upstream_tag in versions.items():
        for arch in arches:
            try:
                upstream = upstream_ids(version, upstream_tag, arch)
            except urllib.error.HTTPError as e:
                if e.code != 404:
                    raise
                # A new minor has no upstream tag until its GA day; skip it
                # rather than stop the check for every other version.
                log(f'PHP {version}: no {upstream_tag}-fpm-alpine upstream yet, skipped')
                break
            try:
                ours = our_ids(version, arch)
            except urllib.error.HTTPError as e:
                if e.code != 404:
                    raise
                ours = []
            if ours[:len(upstream)] != upstream:
                log(f'PHP {version} ({arch}): base is not built on the current {upstream_tag}-fpm-alpine')
                changed.append(version)
                break
        else:
            log(f'PHP {version}: up to date')
    return changed


def main():
    with open(CONTAINERFILE) as f:
        tag = recipe_tag(f.read())

    def upstream_ids(version, upstream_tag, arch):
        return diff_ids('registry-1.docker.io',
                        'https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/php:pull',
                        'library/php', f'{upstream_tag}-fpm-alpine', arch)

    def our_ids(version, arch):
        repo = 'lerd-env/lerd-php' + version.replace('.', '') + '-fpm-base'
        return diff_ids('ghcr.io', f'https://ghcr.io/token?scope=repository:{repo}:pull',
                        repo, f'{tag}-{arch}', arch)

    changed = changed_versions(VERSIONS, ARCHES, upstream_ids, our_ids)
    with open(os.environ['GITHUB_OUTPUT'], 'a') as out:
        out.write(f'changed={",".join(changed)}\n')


if __name__ == '__main__':
    main()
