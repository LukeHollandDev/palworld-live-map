import { spawn } from 'node:child_process'
import { access, copyFile, mkdir, rm, stat } from 'node:fs/promises'
import { createServer } from 'node:net'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright'

const scriptDirectory = dirname(fileURLToPath(import.meta.url))
const projectRoot = resolve(scriptDirectory, '../..')
const buildDirectory = join(projectRoot, 'build/demo-media')
const imageDirectory = join(projectRoot, 'assets/images')
const serverBinary = join(buildDirectory, 'palworld-live-map')
const rawVideo = join(buildDirectory, 'demo.webm')
const posterImage = join(buildDirectory, 'demo.png')
const gifVideo = join(buildDirectory, 'demo.gif')
const captureWidth = 1440
const captureHeight = 900
const maximumGifBytes = 10_000_000

let appProcess
let browser

function appHasExited() {
  return !appProcess || appProcess.exitCode !== null || appProcess.signalCode !== null
}

function run(command, args, options = {}) {
  return new Promise((resolveRun, rejectRun) => {
    const child = spawn(command, args, {
      cwd: projectRoot,
      env: process.env,
      stdio: options.quiet ? ['ignore', 'pipe', 'pipe'] : 'inherit'
    })
    let stderr = ''
    if (options.quiet) child.stderr.on('data', (chunk) => (stderr += chunk))
    child.once('error', rejectRun)
    child.once('exit', (code, signal) => {
      if (code === 0) resolveRun()
      else rejectRun(new Error(`${command} exited with ${signal || code}${stderr ? `: ${stderr.trim()}` : ''}`))
    })
  })
}

async function availablePort() {
  const requestedPort = process.env.DEMO_MEDIA_PORT
  if (requestedPort) {
    const port = Number(requestedPort)
    if (!Number.isInteger(port) || port < 1 || port > 65535)
      throw new Error('DEMO_MEDIA_PORT must be an integer from 1 to 65535')
    return port
  }
  return new Promise((resolvePort, rejectPort) => {
    const probe = createServer()
    probe.unref()
    probe.once('error', rejectPort)
    probe.listen(0, '127.0.0.1', () => {
      const address = probe.address()
      if (!address || typeof address === 'string') {
        probe.close()
        rejectPort(new Error('Could not allocate a local port for the demo server'))
        return
      }
      probe.close((error) => (error ? rejectPort(error) : resolvePort(address.port)))
    })
  })
}

async function waitForServer(url) {
  const deadline = Date.now() + 30_000
  while (Date.now() < deadline) {
    if (appHasExited())
      throw new Error(`Demo server exited with ${appProcess.signalCode || `code ${appProcess.exitCode}`}`)
    try {
      const response = await fetch(`${url}/-/health`)
      if (response.ok) return
    } catch {
      // The Go process may still be starting.
    }
    await new Promise((resolveWait) => setTimeout(resolveWait, 150))
  }
  throw new Error('Timed out waiting for the demo server')
}

async function stopApp() {
  if (appHasExited()) return
  appProcess.kill('SIGTERM')
  await Promise.race([
    new Promise((resolveExit) => appProcess.once('exit', resolveExit)),
    new Promise((resolveWait) => setTimeout(resolveWait, 3_000))
  ])
  if (!appHasExited()) appProcess.kill('SIGKILL')
}

async function settle(page, milliseconds = 700) {
  await page.waitForTimeout(milliseconds)
}

async function hoverControl(page, locator, milliseconds = 350) {
  await locator.hover()
  await settle(page, milliseconds)
}

async function clickControl(page, locator, milliseconds = 350) {
  await hoverControl(page, locator, milliseconds)
  await locator.click()
}

async function checkControl(page, locator, milliseconds = 350) {
  await hoverControl(page, locator, milliseconds)
  await locator.check()
}

async function traceMapCoordinates(page) {
  const map = page.getByRole('application', { name: /interactive world map/i })
  const bounds = await map.boundingBox()
  if (!bounds) throw new Error('Could not locate the interactive map for coordinate tracing')
  await page.mouse.move(bounds.x + bounds.width * 0.48, bounds.y + bounds.height * 0.58, { steps: 18 })
  await settle(page, 450)
  await page.mouse.move(bounds.x + bounds.width * 0.66, bounds.y + bounds.height * 0.44, { steps: 28 })
  await settle(page, 700)
}

async function selectLeaderboard(page, name) {
  await clickControl(page, page.getByRole('button', { name: /Leaderboard type/ }))
  await settle(page, 1_150)
  await clickControl(page, page.getByRole('option', { name, exact: true }), 450)
  await page.getByRole('heading', { name, exact: true }).waitFor()
  await settle(page, 1_900)
}

async function installDemoCursor(page) {
  await page.evaluate(() => {
    const cursor = document.createElement('div')
    cursor.id = 'demo-cursor'
    cursor.setAttribute('aria-hidden', 'true')
    cursor.innerHTML = `<svg viewBox="0 0 24 28" width="24" height="28" aria-hidden="true"><path d="M2 2v21l6-5 4 8 5-2-4-8h8L2 2Z" fill="#f5feff" stroke="#071014" stroke-width="2" stroke-linejoin="round"/></svg>`
    Object.assign(cursor.style, {
      position: 'fixed',
      top: '0',
      left: '0',
      zIndex: '2147483647',
      width: '24px',
      height: '28px',
      opacity: '0',
      pointerEvents: 'none',
      transform: 'translate(-3px, -3px)',
      transition: 'left 180ms ease-out, top 180ms ease-out, opacity 100ms ease-out',
      filter: 'drop-shadow(0 1px 2px rgb(0 0 0 / 80%))'
    })
    document.documentElement.append(cursor)
    const glyph = cursor.firstElementChild
    window.addEventListener(
      'mousemove',
      (event) => {
        cursor.style.left = `${event.clientX}px`
        cursor.style.top = `${event.clientY}px`
        cursor.style.opacity = '1'
      },
      true
    )
    window.addEventListener('mousedown', () => {
      if (glyph instanceof SVGElement) glyph.style.transform = 'scale(.82)'
    })
    window.addEventListener('mouseup', () => {
      if (glyph instanceof SVGElement) glyph.style.transform = ''
    })
  })
}

async function preloadWorldTreeArtwork(page) {
  await page.evaluate(async () => {
    const response = await fetch('/api/config')
    if (!response.ok) throw new Error(`/api/config returned ${response.status}`)
    const config = await response.json()
    const layer = config.layers?.find((candidate) => candidate.id === 'world-tree')
    if (!layer) throw new Error('Demo config does not include World Tree artwork')
    if (layer.tilePyramid) {
      const level = Math.min(...layer.tilePyramid.levels)
      const columns = Math.ceil(level / layer.tilePyramid.tileSize)
      await Promise.all(
        Array.from({ length: columns * columns }, (_, index) => {
          const x = index % columns
          const y = Math.floor(index / columns)
          const image = new Image()
          image.src = layer.tilePyramid.urlTemplate
            .replace('{size}', String(level))
            .replace('{x}', String(x))
            .replace('{y}', String(y))
          return image.decode()
        })
      )
      return
    }
    if (!layer.imageUrl) throw new Error('Demo config does not include World Tree artwork')
    const image = new Image()
    image.src = layer.imageUrl
    await image.decode()
  })
}

async function recordWalkthrough(baseUrl) {
  browser = await chromium.launch({ headless: true })
  const context = await browser.newContext({
    viewport: { width: captureWidth, height: captureHeight },
    deviceScaleFactor: 1,
    colorScheme: 'dark',
    reducedMotion: 'no-preference'
  })
  const page = await context.newPage()
  await page.goto(baseUrl, { waitUntil: 'domcontentloaded' })
  await page.getByRole('heading', { name: 'Palpagos Live Demo', exact: true }).waitFor()
  await page.locator('.map-tile-layer.is-ready').waitFor()
  await preloadWorldTreeArtwork(page)
  await settle(page, 900)
  await page.screenshot({ path: posterImage })
  await installDemoCursor(page)

  await page.screencast.start({
    path: rawVideo,
    size: { width: captureWidth, height: captureHeight },
    quality: 90
  })
  await settle(page, 2_000)
  await traceMapCoordinates(page)

  await clickControl(page, page.getByRole('button', { name: 'Hide all map categories', exact: true }))
  await settle(page, 1_350)
  const search = page.getByRole('searchbox', { name: 'Search map locations and live objects' })
  await hoverControl(page, search)
  await search.fill('Aurora')
  await settle(page, 900)
  await checkControl(page, page.getByRole('checkbox', { name: 'Show Online Players', exact: true }))
  await settle(page, 650)
  await checkControl(page, page.getByRole('checkbox', { name: 'Show Guilds', exact: true }))
  await settle(page, 1_600)
  await clickControl(page, page.getByRole('button', { name: 'View guild Aurora Expedition', exact: true }))
  await page.getByRole('heading', { name: 'Aurora Expedition', exact: true }).waitFor()
  await settle(page, 2_100)
  await clickControl(page, page.getByRole('button', { name: 'View guild base Base 1', exact: true }), 500)
  await page.getByText('BASE DETAILS', { exact: true }).waitFor()
  await page.getByRole('heading', { name: 'Assigned Pals', exact: true }).scrollIntoViewIfNeeded()
  await hoverControl(page, page.getByRole('button', { name: /View assigned Pal Anubis/ }), 600)
  await settle(page, 2_200)
  await clickControl(page, page.getByRole('button', { name: 'Close details', exact: true }))
  await settle(page, 650)
  await clickControl(page, page.getByRole('button', { name: 'Clear search', exact: true }))
  await settle(page, 850)
  await clickControl(page, page.getByRole('button', { name: 'Fit', exact: true }))
  await settle(page, 750)
  await checkControl(page, page.getByRole('checkbox', { name: 'Show Alpha Pals', exact: true }))
  await settle(page, 1_350)
  await checkControl(page, page.getByRole('checkbox', { name: 'Show Waypoints', exact: true }))
  await settle(page, 1_700)

  await clickControl(page, page.getByRole('button', { name: 'My Progress', exact: true }))
  await page.getByRole('heading', { name: 'My Progress', exact: true }).waitFor()
  const breakdown = page.getByText('Breakdown', { exact: true })
  await breakdown.waitFor()
  await hoverControl(page, breakdown, 600)
  await settle(page, 2_200)
  await clickControl(page, page.getByRole('button', { name: 'Close My Progress', exact: true }))
  await settle(page, 900)

  await clickControl(page, page.getByRole('button', { name: 'Leaderboards', exact: true }))
  await page.getByRole('heading', { name: 'Leaderboards', exact: true }).waitFor()
  await settle(page, 1_800)
  await selectLeaderboard(page, 'Total captures')
  await selectLeaderboard(page, 'Paldeck discoveries')
  await selectLeaderboard(page, 'Arena RP')
  await selectLeaderboard(page, 'Boss clears')
  await clickControl(page, page.getByRole('button', { name: 'Close details', exact: true }))
  await settle(page, 900)

  await clickControl(page, page.getByRole('button', { name: 'World Tree', exact: true }))
  await page.locator('button[aria-pressed="true"]').filter({ hasText: 'World Tree' }).waitFor()
  await page.locator('.map-tile-layer.is-ready .map-tile[src*="world-tree"]').first().waitFor()
  await settle(page, 1_200)
  await clickControl(page, page.getByRole('button', { name: 'Show all map categories', exact: true }))
  await settle(page, 1_400)
  await clickControl(page, page.getByRole('button', { name: 'Fit', exact: true }))
  await settle(page, 1_100)
  await traceMapCoordinates(page)

  await clickControl(page, page.locator('.map-marker[aria-label="The Verdant Rootpath"]'), 550)
  await page.getByRole('heading', { name: 'The Verdant Rootpath', exact: true }).waitFor()
  await settle(page, 1_900)
  await clickControl(page, page.getByRole('button', { name: 'Close details', exact: true }))
  await settle(page, 700)
  await clickControl(page, page.getByRole('button', { name: 'Fit', exact: true }))
  await settle(page, 900)
  for (let step = 0; step < 4; step += 1) {
    await clickControl(page, page.getByRole('button', { name: 'Zoom in', exact: true }), 250)
    await settle(page, 550)
  }
  await settle(page, 800)
  await clickControl(page, page.locator('.map-marker[aria-label="Orbit · Lv 60"]'), 550)
  await page.getByRole('heading', { name: 'Orbit', exact: true }).waitFor()
  await page.getByRole('heading', { name: 'Current companion Pals', exact: true }).scrollIntoViewIfNeeded()
  await hoverControl(page, page.getByRole('button', { name: /View companion Pal Xenolord/ }), 600)
  await settle(page, 2_200)

  await page.screencast.stop()
  await context.close()
}

async function encodeMedia() {
  await run('ffmpeg', [
    '-hide_banner',
    '-loglevel',
    'error',
    '-y',
    '-i',
    rawVideo,
    '-filter_complex',
    'fps=4,scale=1000:-2:flags=lanczos,split[frames][paletteframes];[paletteframes]palettegen=max_colors=28:stats_mode=diff[palette];[frames][palette]paletteuse=dither=bayer:bayer_scale=3:diff_mode=rectangle',
    '-loop',
    '0',
    gifVideo
  ])
  const gif = await stat(gifVideo)
  if (gif.size > maximumGifBytes)
    throw new Error(`Generated GIF is ${(gif.size / 1_000_000).toFixed(1)} MB; expected at most 10 MB`)
}

async function publishMedia() {
  await mkdir(imageDirectory, { recursive: true })
  await Promise.all([
    copyFile(posterImage, join(imageDirectory, 'demo.png')),
    copyFile(gifVideo, join(imageDirectory, 'demo.gif'))
  ])
}

async function main() {
  await run('ffmpeg', ['-version'], { quiet: true })
  await rm(buildDirectory, { recursive: true, force: true })
  await mkdir(buildDirectory, { recursive: true })
  await run('go', ['build', '-o', serverBinary, './cmd/palworld-live-map'])
  await access(serverBinary)
  const port = await availablePort()
  const baseUrl = `http://127.0.0.1:${port}`
  appProcess = spawn(serverBinary, [], {
    cwd: projectRoot,
    env: {
      ...process.env,
      ADDR: `127.0.0.1:${port}`,
      DEMO_MODE: 'true',
      POLL_INTERVAL: '2s',
      UPSTREAM_TIMEOUT: '1s',
      WORLD_POLL_INTERVAL: '5s',
      WORLD_TIMEOUT: '4s'
    },
    stdio: ['ignore', 'inherit', 'inherit']
  })
  await waitForServer(baseUrl)
  await recordWalkthrough(baseUrl)
  await encodeMedia()
  await publishMedia()
  const gif = await stat(gifVideo)
  console.log(`Generated assets/images/demo.gif (${(gif.size / 1024 / 1024).toFixed(1)} MiB)`)
}

try {
  await main()
} finally {
  if (browser) await browser.close()
  await stopApp()
}
