import unittest

from ai_attribution import offending


class OffendingTest(unittest.TestCase):
    def test_flags_an_ai_co_author(self):
        text = 'fix: thing\n\nBody.\n\nCo-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>\n'
        self.assertEqual(offending(text), ['Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>'])

    def test_flags_a_generated_with_footer(self):
        text = 'Body.\n\n🤖 Generated with [Claude Code](https://claude.com/claude-code)\n'
        self.assertEqual(len(offending(text)), 1)

    def test_flags_copilot_in_any_case(self):
        self.assertEqual(len(offending('co-authored-by: Copilot <175728472+Copilot@users.noreply.github.com>')), 1)

    def test_lets_a_human_co_author_through(self):
        self.assertEqual(offending('Co-authored-by: Jane Doe <jane@example.test>'), [])

    def test_lets_prose_mentioning_claude_through(self):
        self.assertEqual(offending('Boost writes the MCP config Claude Code reads.'), [])


if __name__ == '__main__':
    unittest.main()
