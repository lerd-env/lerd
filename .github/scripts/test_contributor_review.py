import json
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


if __name__ == '__main__':
    unittest.main()
