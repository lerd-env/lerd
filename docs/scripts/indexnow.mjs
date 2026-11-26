// Reads changed repo paths on stdin (git diff --name-only) and submits the
// pages they publish to IndexNow. Run after the site has been deployed.
import { readFileSync } from 'node:fs'
import { changedUrls, submission } from '../.vitepress/indexnow.js'

const urls = changedUrls(readFileSync(0, 'utf8').split('\n'))
if (urls.length === 0) {
  console.log('IndexNow: no page changed, nothing to submit')
  process.exit(0)
}

const res = await fetch('https://api.indexnow.org/indexnow', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json; charset=utf-8' },
  body: JSON.stringify(submission(urls)),
})
console.log(`IndexNow: ${res.status} for ${urls.length} URLs\n${urls.join('\n')}`)
if (!res.ok) {
  console.error(await res.text())
  process.exit(1)
}
