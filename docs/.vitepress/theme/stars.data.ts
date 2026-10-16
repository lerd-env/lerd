import { defineLoader } from 'vitepress'

// The count is read once while the docs are built, so the number is in the HTML
// the visitor receives rather than arriving a moment later and shifting the
// button under their cursor. A build with no network answers 0, and the button
// renders without a count rather than with a wrong one.
export interface Data {
  stars: number
}

declare const data: Data
export { data }

export default defineLoader({
  async load(): Promise<Data> {
    try {
      const res = await fetch('https://api.github.com/repos/lerd-env/lerd', {
        headers: { Accept: 'application/vnd.github+json' },
      })
      if (!res.ok) return { stars: 0 }
      const json = await res.json()
      return { stars: Number(json.stargazers_count) || 0 }
    } catch {
      return { stars: 0 }
    }
  },
})
