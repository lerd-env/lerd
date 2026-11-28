import unittest

from contributor_review import should_request, team_requested


def pr(draft=False, user_type='User'):
    return {'draft': draft, 'user': {'type': user_type}}


class ShouldRequestTest(unittest.TestCase):
    def test_requests_on_a_ready_human_pr(self):
        self.assertTrue(should_request(pr()))

    def test_skips_a_draft(self):
        self.assertFalse(should_request(pr(draft=True)))

    def test_skips_a_bot(self):
        self.assertFalse(should_request(pr(user_type='Bot')))


class TeamRequestedTest(unittest.TestCase):
    def test_sees_the_team_among_the_requested(self):
        response = {'requested_teams': [{'slug': 'maintainers'}, {'slug': 'contributors'}]}
        self.assertTrue(team_requested(response))

    def test_catches_a_request_github_silently_dropped(self):
        self.assertFalse(team_requested({'requested_teams': [{'slug': 'maintainers'}]}))

    def test_catches_a_response_with_no_teams(self):
        self.assertFalse(team_requested({}))


if __name__ == '__main__':
    unittest.main()
