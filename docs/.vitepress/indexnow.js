// IndexNow proves ownership with a public key file, so the key is not a secret.
// It must match docs/public/<KEY>.txt.
const KEY = 'f34919ee8997f86c61d202ff3dda3dba'
const HOST = 'lerd.sh'
const SITE_URL = `https://${HOST}`

// Turns changed repo paths into the lerd.sh URLs they publish, in the same
// clean-URL form the sitemap uses. Anything that is not a page is dropped.
export function changedUrls(files) {
  const urls = []
  for (const file of files) {
    const digest = file.match(/^docs\/public\/(digest\/[^/]+\.html)$/)
    if (digest) {
      urls.push(`${SITE_URL}/${digest[1]}`)
      continue
    }
    const page = file.match(/^docs\/(.+)\.md$/)
    if (!page || page[1].startsWith('node_modules/') || page[1] === 'changelog-archive') continue
    urls.push(`${SITE_URL}/${page[1].replace(/(^|\/)index$/, '$1')}`)
  }
  return urls
}

export function submission(urlList) {
  return { host: HOST, key: KEY, keyLocation: `${SITE_URL}/${KEY}.txt`, urlList }
}
