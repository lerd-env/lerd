import json
import os
import shlex
import subprocess
import sys
import tempfile
import unittest

from contributor_review import problem, should_request


def pr(draft=False, user_type='User'):
    return {'draft': draft, 'user': {'type': user_type}}


class ShouldRequestTest(unittest.TestCase):
    def test_requests_on_a_ready_human_pr(self):
        self.assertTrue(should_request(pr()))

    def test_skips_a_draft(self):
        self.assertFalse(should_request(pr(draft=True)))

    def test_skips_a_bot(self):
        self.assertFalse(should_request(pr(user_type='Bot')))


class ProblemTest(unittest.TestCase):
    def test_none_when_the_team_was_added(self):
        out = json.dumps({'requested_teams': [{'slug': 'maintainers'}, {'slug': 'contributors'}]})
        self.assertIsNone(problem(0, out, ''))

    def test_reports_githubs_reason_when_refused(self):
        msg = problem(1, '', 'gh: Resource not accessible by integration (HTTP 403)')
        self.assertIn('Resource not accessible by integration', msg)

    def test_reports_a_request_github_silently_dropped(self):
        out = json.dumps({'requested_teams': [{'slug': 'maintainers'}]})
        self.assertIn('was not added', problem(0, out, ''))


SCRIPT = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'contributor_review.py')


class ScriptTest(unittest.TestCase):
    """Runs the script against a fake gh, so a missed request is proven to warn, not fail."""

    def run_script(self, token='t', gh_exit=0, gh_out='', gh_err=''):
        with tempfile.TemporaryDirectory() as d:
            gh = os.path.join(d, 'gh')
            with open(gh, 'w') as f:
                f.write(f'#!/bin/sh\nprintf %s {shlex.quote(gh_out)}\nprintf %s {shlex.quote(gh_err)} >&2\nexit {gh_exit}\n')
            os.chmod(gh, 0o755)
            event = os.path.join(d, 'event.json')
            with open(event, 'w') as f:
                json.dump({'pull_request': {'number': 1, **pr()}}, f)
            env = {'PATH': f"{d}:{os.environ['PATH']}", 'GITHUB_EVENT_PATH': event,
                   'GITHUB_REPOSITORY': 'o/r', 'GH_TOKEN': token}
            return subprocess.run([sys.executable, SCRIPT], env=env, capture_output=True, text=True)

    def test_missing_token_warns_and_passes(self):
        run = self.run_script(token='')
        self.assertEqual(run.returncode, 0)
        self.assertIn('::warning::CONTRIBUTOR_REVIEW_TOKEN', run.stdout)

    def test_refused_request_warns_with_githubs_reason_and_passes(self):
        run = self.run_script(gh_exit=1, gh_err='gh: Not Found (HTTP 404)')
        self.assertEqual(run.returncode, 0)
        self.assertIn('::warning::', run.stdout)
        self.assertIn('Not Found (HTTP 404)', run.stdout)

    def test_added_team_reports_success(self):
        run = self.run_script(gh_out=json.dumps({'requested_teams': [{'slug': 'contributors'}]}))
        self.assertEqual(run.returncode, 0)
        self.assertNotIn('::warning::', run.stdout)


if __name__ == '__main__':
    unittest.main()
