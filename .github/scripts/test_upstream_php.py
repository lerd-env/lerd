import unittest
import urllib.error

import upstream_php


def http_error(code):
    return urllib.error.HTTPError('https://registry.example/v2/x', code, 'error', {}, None)


class ChangedVersions(unittest.TestCase):
    def run_with(self, upstream, ours, versions=None):
        """upstream and ours map (version, arch) to layer ids or to an HTTP code."""
        def lookup(table, key):
            got = table[key]
            if isinstance(got, int):
                raise http_error(got)
            return got

        lines = []
        changed = upstream_php.changed_versions(
            versions or {'8.5': '8.5', '8.6': '8.6', '8.7': '8.7'},
            ('amd64',),
            lambda version, tag, arch: lookup(upstream, (version, arch)),
            lambda version, arch: lookup(ours, (version, arch)),
            lines.append,
        )
        return changed, lines

    def test_a_base_built_on_the_current_upstream_is_up_to_date(self):
        changed, lines = self.run_with({('8.5', 'amd64'): ['a', 'b']}, {('8.5', 'amd64'): ['a', 'b', 'lerd']}, {'8.5': '8.5'})
        self.assertEqual(changed, [])
        self.assertEqual(lines, ['PHP 8.5: up to date'])

    def test_a_republished_upstream_marks_the_version_changed(self):
        changed, _ = self.run_with({('8.5', 'amd64'): ['a', 'new']}, {('8.5', 'amd64'): ['a', 'b', 'lerd']}, {'8.5': '8.5'})
        self.assertEqual(changed, ['8.5'])

    def test_a_base_never_published_counts_as_changed(self):
        changed, _ = self.run_with({('8.5', 'amd64'): ['a']}, {('8.5', 'amd64'): 404}, {'8.5': '8.5'})
        self.assertEqual(changed, ['8.5'])

    def test_a_missing_upstream_skips_only_that_version(self):
        changed, lines = self.run_with(
            {('8.5', 'amd64'): ['a'], ('8.6', 'amd64'): 404, ('8.7', 'amd64'): ['new']},
            {('8.5', 'amd64'): ['a', 'lerd'], ('8.7', 'amd64'): ['old', 'lerd']},
        )
        self.assertEqual(changed, ['8.7'])
        self.assertIn('PHP 8.6: no 8.6-fpm-alpine upstream yet, skipped', lines)
        self.assertIn('PHP 8.5: up to date', lines)

    def test_any_other_upstream_error_still_fails_the_check(self):
        with self.assertRaises(urllib.error.HTTPError):
            self.run_with({('8.5', 'amd64'): 500}, {('8.5', 'amd64'): ['a']}, {'8.5': '8.5'})


if __name__ == '__main__':
    unittest.main()
