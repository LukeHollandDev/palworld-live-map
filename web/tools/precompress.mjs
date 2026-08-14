import { readdir, readFile, writeFile } from 'node:fs/promises'
import { extname, join } from 'node:path'
import { promisify } from 'node:util'
import { brotliCompress, constants, gzip } from 'node:zlib'

const brotli = promisify(brotliCompress)
const gzipAsync = promisify(gzip)
const root = new URL('../dist', import.meta.url).pathname

async function filesUnder(directory) {
  const entries = await readdir(directory, { withFileTypes: true })
  const files = []
  for (const entry of entries.sort((left, right) => left.name.localeCompare(right.name))) {
    const filename = join(directory, entry.name)
    if (entry.isDirectory()) files.push(...(await filesUnder(filename)))
    else if (entry.isFile() && ['.js', '.css'].includes(extname(entry.name))) files.push(filename)
  }
  return files
}

for (const filename of await filesUnder(root)) {
  const source = await readFile(filename)
  const [brotliBytes, gzipBytes] = await Promise.all([
    brotli(source, {
      params: {
        [constants.BROTLI_PARAM_QUALITY]: 6,
        [constants.BROTLI_PARAM_MODE]: constants.BROTLI_MODE_TEXT
      }
    }),
    gzipAsync(source, { level: 9, mtime: 0 })
  ])
  await Promise.all([writeFile(`${filename}.br`, brotliBytes), writeFile(`${filename}.gz`, gzipBytes)])
}
