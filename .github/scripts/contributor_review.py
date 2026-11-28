"""Request a review from the contributors team on a ready, human-opened pull request."""
import json
import os
import subprocess
import sys

TEAM = 'contributors'


def should_request(pr):
    return not pr['draft'] and pr['user']['type'] != 'Bot'


def team_requested(response):
    return any(team['slug'] == TEAM for team in response.get('requested_teams', []))


if __name__ == '__main__':
    with open(os.environ['GITHUB_EVENT_PATH']) as f:
        pr = json.load(f)['pull_request']
    if not should_request(pr):
        print('Draft or bot pull request, no review requested.')
        sys.exit(0)
    out = subprocess.run(
        ['gh', 'api', '-X', 'POST', f"repos/{os.environ['GITHUB_REPOSITORY']}/pulls/{pr['number']}/requested_reviewers",
         '-f', f'team_reviewers[]={TEAM}'],
        check=True, capture_output=True, text=True).stdout
    if not team_requested(json.loads(out)):
        print(f'::error::the {TEAM} team was not added as a reviewer')
        sys.exit(1)
    print(f'Requested a review from {TEAM}.')
