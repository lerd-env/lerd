import hashlib
import os
import unittest
from unittest import mock

import bump_tool_pins as b

MANIFEST = '''# header comment
tools:
  composer:
    version: "2.10.0"
    url: https://getcomposer.org/download/{version}/composer.phar
  # a comment between tools
  mise:
    version: v1.0.0
    url: https://example.test/{version}/{asset}
    assets:
      linux/amd64: mise-{version}-linux-x64.tar.gz
      darwin/arm64: mise-{version}-macos-arm64.tar.gz
    digests:
      linux/amd64: old
      darwin/arm64: old
  php-host-8.4:
    version: "8.4.1"
    url: https://example.test/{asset}
    assets:
      linux/amd64: php-{version}-cli-linux-x86_64.tar.gz
    digests:
      linux/amd64: old
    sizes:
      linux/amd64: 1
'''


def lines():
    return MANIFEST.split('\n')


def php(version, *platforms):
    return [{'name': f'php-{version}-cli-{p}.tar.gz'} for p in platforms]


ALL = ('linux-x86_64', 'linux-aarch64', 'macos-x86_64', 'macos-aarch64')


class NewestPHPPatch(unittest.TestCase):
    def test_orders_patches_numerically(self):
        listing = php('8.4.9', *ALL) + php('8.4.10', *ALL) + php('8.3.30', *ALL)
        self.assertEqual(b.newest_php_patch(listing, '8.4'), '8.4.10')

    def test_skips_a_patch_missing_a_platform(self):
        listing = php('8.4.9', *ALL) + php('8.4.10', *ALL[:3])
        self.assertEqual(b.newest_php_patch(listing, '8.4'), '8.4.9')

    def test_does_not_match_another_minor_sharing_the_prefix(self):
        self.assertIsNone(b.newest_php_patch(php('8.40.1', *ALL), '8.4'))


class Bump(unittest.TestCase):
    def test_an_unchanged_pin_downloads_nothing(self):
        got = lines()
        self.assertFalse(b.bump(got, 'mise', 'v1.0.0', lambda url: self.fail(url)))
        self.assertEqual(got, lines())

    def test_recomputes_every_platform_digest_from_the_new_asset(self):
        served = {}

        def download(url):
            served[url] = url.encode()
            return served[url]

        got = lines()
        self.assertTrue(b.bump(got, 'mise', 'v2.0.0', download))
        self.assertEqual(sorted(served), ['https://example.test/v2.0.0/mise-v2.0.0-linux-x64.tar.gz',
                                          'https://example.test/v2.0.0/mise-v2.0.0-macos-arm64.tar.gz'])
        index = b.parse(got)
        self.assertEqual(b.get(got, index, 'mise', 'version'), 'v2.0.0')
        for platform, url in (('linux/amd64', sorted(served)[0]), ('darwin/arm64', sorted(served)[1])):
            self.assertEqual(b.get(got, index, 'mise', 'digests', platform), hashlib.sha256(served[url]).hexdigest())

    def test_rewrites_sizes_and_keeps_quotes_and_comments(self):
        got = lines()
        b.bump(got, 'php-host-8.4', '8.4.2', lambda url: b'12345')
        text = '\n'.join(got)
        self.assertIn('    version: "8.4.2"\n', text)
        self.assertIn('      linux/amd64: 5\n', text)
        self.assertIn('  # a comment between tools\n', text)
        self.assertIn('# header comment\n', text)

    def test_a_pin_without_digests_only_moves_its_version(self):
        got = lines()
        b.bump(got, 'composer', '2.11.0', lambda url: self.fail(url))
        self.assertEqual(['    version: "2.11.0"' if l == '    version: "2.10.0"' else l for l in lines()], got)


class Fetch(unittest.TestCase):
    def response(self, body):
        resp = mock.MagicMock()
        resp.__enter__.return_value.read.return_value = body
        return resp

    @mock.patch('time.sleep')
    def test_retries_a_transient_failure(self, _sleep):
        with mock.patch('urllib.request.urlopen', side_effect=[OSError('reset'), self.response(b'ok')]):
            self.assertEqual(b.fetch('https://example.test/a'), b'ok')

    @mock.patch('time.sleep')
    def test_gives_up_after_the_last_attempt(self, _sleep):
        with mock.patch('urllib.request.urlopen', side_effect=OSError('down')) as urlopen:
            with self.assertRaises(OSError):
                b.fetch('https://example.test/a')
        self.assertEqual(urlopen.call_count, 3)


class RealManifest(unittest.TestCase):
    def test_every_pin_resolves_to_an_upstream_lookup(self):
        path = os.path.join(os.path.dirname(__file__), '..', '..', b.FILE)
        with open(path) as f:
            index = b.parse(f.read().split('\n'))
        tools = [k[0] for k in index if len(k) == 1]
        self.assertIn('php-host-8.4', tools)
        for tool in tools:
            self.assertTrue(tool == 'composer' or tool in b.GITHUB or tool.startswith('php-host-'), tool)
            self.assertIn((tool, 'version'), index)


if __name__ == '__main__':
    unittest.main()
