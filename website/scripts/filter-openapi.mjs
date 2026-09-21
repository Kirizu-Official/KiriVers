/**
 * Copy unfiltered plane OpenAPI JSON into website/docs/public/openapi/.
 * Source files live in the Go tree; this script must fail if they are missing.
 * Output names match the plane files: openapi.client.json / openapi.admin.json.
 */
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const websiteRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const repoRoot = path.resolve(websiteRoot, '..')
const outDir = path.join(websiteRoot, 'docs', 'public', 'openapi')

const copies = [
  {
    src: path.join(repoRoot, 'internal', 'controller', 'openapi.client.json'),
    dest: 'openapi.client.json',
    label: 'client OpenAPI',
  },
  {
    src: path.join(repoRoot, 'internal', 'controller', 'openapi.admin.json'),
    dest: 'openapi.admin.json',
    label: 'admin OpenAPI',
  },
]

const leftoverSlices = ['client.json', 'store.json', 'admin.json', 'ci.json']

fs.mkdirSync(outDir, { recursive: true })

for (const { src, dest, label } of copies) {
  if (!fs.existsSync(src)) {
    console.error(`Missing ${label}: ${src}`)
    process.exit(1)
  }
  fs.copyFileSync(src, path.join(outDir, dest))
}

for (const name of leftoverSlices) {
  const leftover = path.join(outDir, name)
  if (fs.existsSync(leftover)) {
    fs.unlinkSync(leftover)
  }
}

console.log(`Copied ${copies.map((c) => c.dest).join(', ')} to ${outDir}`)
