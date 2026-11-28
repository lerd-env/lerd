"""Request a review from the contributors team on a ready, human-opened pull request."""
import json
import os
import subprocess
import sys

TEAM = 'contributors'


def should_request(pr):
    return not pr['draft'] and pr['user']['type'] != 'Bot'


def problem(returncode, stdout, stderr):
    """Why the team request did not land, or None when it did."""
    if returncode != 0:
        return f'GitHub refused to request the {TEAM} team: {stderr.strip()}'
    if not any(t['slug'] == TEAM for t in json.loads(stdout).get('requested_teams', [])):
        return f'the {TEAM} team was not added as a reviewer'
    return None


# A missed request only costs a notification, so it warns instead of failing the PR's checks.
if __name__ == '__main__':
    with open(os.environ['GITHUB_EVENT_PATH']) as f:
        pr = json.load(f)['pull_request']
    if not should_request(pr):
        print('Draft or bot pull request, no review requested.')
        sys.exit(0)
    if not os.environ.get('GH_TOKEN'):
        print('::warning::CONTRIBUTOR_REVIEW_TOKEN secret is not set, no review requested.')
        sys.exit(0)
    run = subprocess.run(
        ['gh', 'api', '-X', 'POST', f"repos/{os.environ['GITHUB_REPOSITORY']}/pulls/{pr['number']}/requested_reviewers",
         '-f', f'team_reviewers[]={TEAM}'],
        capture_output=True, text=True)
    msg = problem(run.returncode, run.stdout, run.stderr)
    print(f'::warning::{msg}' if msg else f'Requested a review from {TEAM}.')
