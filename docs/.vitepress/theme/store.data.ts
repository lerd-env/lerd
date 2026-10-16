import { defineLoader } from 'vitepress'
import { readdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

// The hero's framework and service counts, read from the same store indexes
// lerd itself fetches, so they move with the store instead of the copy. A build
// with no network answers 0 and the page drops that stat rather than show a wrong one.
export interface Data {
  frameworks: number
  services: number
}

declare const data: Data
export { data }

const STORE = 'https://raw.githubusercontent.com/lerd-env'

async function count(url: string, key: string): Promise<number> {
  try {
    const res = await fetch(url)
    if (!res.ok) return 0
    const list = (await res.json())[key]
    return Array.isArray(list) ? list.length : 0
  } catch {
    return 0
  }
}

// The default stack ships inside the binary rather than the store.
function builtinServices(): number {
  const dir = fileURLToPath(new URL('../../../internal/config/presets/', import.meta.url))
  return readdirSync(dir).filter((f) => f.endsWith('.yaml')).length
}

export default defineLoader({
  async load(): Promise<Data> {
    const [frameworks, store] = await Promise.all([
      count(`${STORE}/frameworks/main/frameworks/index.json`, 'frameworks'),
      count(`${STORE}/services/main/services/index.json`, 'services'),
    ])
    return { frameworks, services: store ? store + builtinServices() : 0 }
  },
})
