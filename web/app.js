(() => {
  'use strict'

  const SCENE_SIZE = 1000
  const kinds = ['players', 'bases', 'workers', 'companions', 'wild-pals', 'npcs']
  const enabledKinds = new Set(['players', 'bases'])

  const siteTitle = document.querySelector('#siteTitle')
  const serverDescription = document.querySelector('#serverDescription')
  const statusDot = document.querySelector('#statusDot')
  const statusText = document.querySelector('#statusText')
  const updatedText = document.querySelector('#updatedText')
  const toggleFilters = document.querySelector('#toggleFilters')
  const closeFilters = document.querySelector('#closeFilters')
  const layerTabs = document.querySelector('#layerTabs')
  const filterPanel = document.querySelector('#filterPanel')
  const searchInput = document.querySelector('#searchInput')
  const levelFilter = document.querySelector('#levelFilter')
  const legend = document.querySelector('#legend')
  const objectNotice = document.querySelector('#objectNotice')
  const mapViewport = document.querySelector('#mapViewport')
  const mapScene = document.querySelector('#mapScene')
  const mapImage = document.querySelector('#mapImage')
  const markerLayer = document.querySelector('#markerLayer')
  const objectMarkerLayer = document.querySelector('#objectMarkerLayer')
  const playerMarkerLayer = document.querySelector('#playerMarkerLayer')
  const mapNotice = document.querySelector('#mapNotice')
  const cursorCoordinates = document.querySelector('#cursorCoordinates')

  let config = null
  let playerState = null
  let objectState = emptyObjectState(false)
  let activeLayer = null
  let selectedMarkerKey = null
  let selectedMarkerLayer = null
  let view = { scale: 1, x: 0, y: 0 }
  let drag = null

  function emptyObjectState(enabled) {
    return { enabled, available: false, stale: false, unsupported: false, objects: [] }
  }

  async function requestJSON(path) {
    const response = await fetch(path, { cache: 'no-store' })
    if (!response.ok) throw new Error(`${path} returned ${response.status}`)
    return response.json()
  }

  async function boot() {
    try {
      config = await requestJSON('/api/config')
      objectState = emptyObjectState(config.worldDataEnabled)
      activeLayer = config.layers[0]
      buildLayerTabs()
      resetView()
      renderObjectAvailability()
      renderLegendCounts()
      void refreshPlayers()
      if (config.worldDataEnabled) void refreshObjects()
    } catch {
      setStatus('offline', 'Map unavailable')
    }
  }

  async function refreshPlayers() {
    try {
      playerState = await requestJSON('/api/players')
      renderPlayerState()
    } catch {
      setStatus('offline', 'Map unavailable')
    } finally {
      window.setTimeout(refreshPlayers, config?.pollIntervalMs || 5000)
    }
  }

  async function refreshObjects() {
    try {
      objectState = await requestJSON('/api/objects')
      renderObjectAvailability()
      renderLegendCounts()
      renderObjectMarkers()
    } catch {
      objectNotice.hidden = false
      objectNotice.textContent = objectState.available || objectState.stale
        ? 'World object refresh failed; showing the last received snapshot.'
        : 'World objects are temporarily unavailable.'
    } finally {
      window.setTimeout(refreshObjects, config?.worldPollIntervalMs || 15000)
    }
  }

  function renderPlayerState() {
    renderServerInfo()
    const playerCount = playerState.players?.length || 0
    if (playerState.connected && !playerState.stale) setStatus('live', `${playerCount} player${playerCount === 1 ? '' : 's'} online`)
    else if (playerState.stale) setStatus('stale', `${playerCount} last known`)
    else setStatus('offline', 'Server unavailable')
    renderLegendCounts()
    renderPlayerMarkers()
    updateAge()
  }

  function renderServerInfo() {
    const server = playerState?.server
    if (!server?.name) return
    document.title = server.name
    siteTitle.textContent = server.name
    serverDescription.textContent = server.description || ''
    serverDescription.hidden = !server.description
    serverDescription.title = server.version ? `Palworld ${server.version}` : ''
  }

  function setStatus(kind, text) {
    const className = `status-dot ${kind}`
    if (statusDot.className !== className) statusDot.className = className
    if (statusText.textContent !== text) statusText.textContent = text
  }

  function playerItems() {
    return (playerState?.players || []).map((player) => ({ ...player, kind: 'players', detail: `Level ${player.level}` }))
  }

  function objectItems() {
    return objectState.objects || []
  }

  function allItems() {
    return playerItems().concat(objectItems())
  }

  function filteredItems(items) {
    const query = searchInput.value.trim().toLowerCase()
    const minimumLevel = Number(levelFilter.value)
    return items.filter((item) => {
      if (item.map !== activeLayer.id || !enabledKinds.has(item.kind)) return false
      if ((item.level || 0) < minimumLevel) return false
      if (query && !`${item.name} ${item.detail || ''}`.toLowerCase().includes(query)) return false
      return true
    })
  }

  function renderObjectAvailability() {
    const enabled = config?.worldDataEnabled && objectState.enabled
    const available = enabled && (objectState.available || objectState.stale)
    for (const label of legend.querySelectorAll('label')) {
      const input = label.querySelector('input')
      if (input.value === 'players') continue
      input.disabled = !available
      label.classList.toggle('disabled', !available)
    }

    objectNotice.hidden = false
    if (!enabled) {
      objectNotice.textContent = 'Extra live layers are disabled by this map’s configuration.'
    } else if (objectState.unsupported) {
      objectNotice.textContent = 'Extra live layers need ENABLE_GAMEDATA_API=true and a Palworld server restart.'
    } else if (objectState.stale) {
      objectNotice.textContent = 'World objects are using the last successful snapshot.'
    } else if (!objectState.available) {
      objectNotice.textContent = 'Loading bases, Pals and NPCs…'
    } else {
      objectNotice.hidden = true
    }
  }

  function renderLegendCounts() {
    const counts = Object.fromEntries(kinds.map((kind) => [kind, 0]))
    for (const item of allItems()) {
      if (item.map === activeLayer.id && counts[item.kind] !== undefined) counts[item.kind]++
    }
    for (const label of legend.querySelectorAll('label')) {
      const kind = label.dataset.kind
      label.querySelector('output').textContent = String(counts[kind])
    }
  }

  function renderPlayerMarkers() {
    renderMarkerGroup(playerMarkerLayer, playerItems())
  }

  function renderObjectMarkers() {
    renderMarkerGroup(objectMarkerLayer, objectItems())
  }

  function renderMarkers() {
    renderPlayerMarkers()
    renderObjectMarkers()
  }

  function renderMarkerGroup(container, items) {
    const focusedMarker = container.contains(document.activeElement) ? document.activeElement.closest('.map-marker') : null
    const focusedKey = focusedMarker?.dataset.markerKey || null
    const replacingSelection = selectedMarkerLayer === container.id
    const occurrences = new Map()
    const fragment = document.createDocumentFragment()

    for (const item of filteredItems(items)) {
      const position = toScene(item, activeLayer)
      if (!position) continue
      const baseKey = JSON.stringify([item.kind, item.map, item.name, item.detail || ''])
      const occurrence = occurrences.get(baseKey) || 0
      occurrences.set(baseKey, occurrence + 1)

      const marker = document.createElement('button')
      marker.type = 'button'
      marker.className = `map-marker ${item.kind}`
      marker.dataset.markerKey = `${baseKey}:${occurrence}`
      marker.style.left = `${position.x}px`
      marker.style.top = `${position.y}px`
      marker.style.setProperty('--marker-inverse', String(1 / view.scale))
      marker.setAttribute('aria-label', markerText(item))

      const label = document.createElement('span')
      label.className = 'marker-label'
      label.textContent = markerText(item)
      marker.append(label)
      fragment.append(marker)
    }

    container.replaceChildren(fragment)

    if (selectedMarkerKey) {
      const selected = findMarker(selectedMarkerLayer, selectedMarkerKey)
      if (selected) selected.classList.add('selected')
      else if (replacingSelection) clearSelection()
    }
    if (focusedKey) {
      const focused = findMarker(container.id, focusedKey)
      if (focused) focused.focus({ preventScroll: true })
    }
  }

  function findMarker(layerID, key) {
    const layer = document.getElementById(layerID)
    if (!layer) return null
    return Array.from(layer.querySelectorAll('.map-marker')).find((marker) => marker.dataset.markerKey === key) || null
  }

  function clearSelection() {
    const selected = selectedMarkerKey ? findMarker(selectedMarkerLayer, selectedMarkerKey) : null
    if (selected) selected.classList.remove('selected')
    selectedMarkerKey = null
    selectedMarkerLayer = null
  }

  function markerText(item) {
    const detail = item.detail || (item.level ? `Level ${item.level}` : '')
    return detail ? `${item.name} · ${detail}` : item.name
  }

  function buildLayerTabs() {
    layerTabs.replaceChildren()
    for (const layer of config.layers) {
      const button = document.createElement('button')
      button.type = 'button'
      button.textContent = layer.name
      button.className = layer.id === activeLayer.id ? 'active' : ''
      button.setAttribute('aria-pressed', String(layer.id === activeLayer.id))
      button.addEventListener('click', () => {
        if (layer.id === activeLayer.id) return
        activeLayer = layer
        clearSelection()
        buildLayerTabs()
        resetView()
        renderLegendCounts()
        renderMarkers()
      })
      layerTabs.append(button)
    }
    loadLayerImage()
  }

  function loadLayerImage() {
    mapImage.removeAttribute('src')
    mapImage.hidden = true
    mapNotice.hidden = true
    if (!activeLayer.imageUrl) {
      mapNotice.hidden = false
      return
    }
    mapImage.onload = () => {
      mapImage.hidden = false
      mapNotice.hidden = true
    }
    mapImage.onerror = () => {
      mapImage.hidden = true
      mapNotice.hidden = false
    }
    mapImage.src = activeLayer.imageUrl
  }

  function toScene(item, layer) {
    const [maxX, maxY, minX, minY] = layer.bounds
    if (item.x < minX || item.x > maxX || item.y < minY || item.y > maxY) return null
    return {
      x: ((item.y - minY) / (maxY - minY)) * SCENE_SIZE,
      y: ((maxX - item.x) / (maxX - minX)) * SCENE_SIZE
    }
  }

  function fitScale() {
    const rect = mapViewport.getBoundingClientRect()
    return Math.max(0.01, Math.min(rect.width / SCENE_SIZE, rect.height / SCENE_SIZE))
  }

  function resetView() {
    if (!activeLayer) return
    const rect = mapViewport.getBoundingClientRect()
    if (!rect.width || !rect.height) return
    const scale = fitScale()
    view = { scale, x: (rect.width - SCENE_SIZE * scale) / 2, y: (rect.height - SCENE_SIZE * scale) / 2 }
    applyView()
  }

  function clampView() {
    const rect = mapViewport.getBoundingClientRect()
    const size = SCENE_SIZE * view.scale
    view.x = size <= rect.width ? (rect.width - size) / 2 : Math.min(0, Math.max(rect.width - size, view.x))
    view.y = size <= rect.height ? (rect.height - size) / 2 : Math.min(0, Math.max(rect.height - size, view.y))
  }

  function applyView() {
    mapScene.style.transform = `translate(${view.x}px, ${view.y}px) scale(${view.scale})`
    for (const marker of markerLayer.querySelectorAll('.map-marker')) marker.style.setProperty('--marker-inverse', String(1 / view.scale))
  }

  function zoomAt(nextScale, clientX, clientY) {
    const rect = mapViewport.getBoundingClientRect()
    const minimum = fitScale()
    nextScale = Math.min(minimum * 12, Math.max(minimum, nextScale))
    const pointerX = clientX - rect.left
    const pointerY = clientY - rect.top
    const sceneX = (pointerX - view.x) / view.scale
    const sceneY = (pointerY - view.y) / view.scale
    view.x = pointerX - sceneX * nextScale
    view.y = pointerY - sceneY * nextScale
    view.scale = nextScale
    clampView()
    applyView()
  }

  markerLayer.addEventListener('pointerdown', (event) => {
    if (event.target.closest('.map-marker')) event.stopPropagation()
  })
  markerLayer.addEventListener('click', (event) => {
    const marker = event.target.closest('.map-marker')
    if (!marker || !markerLayer.contains(marker)) return
    event.stopPropagation()
    clearSelection()
    selectedMarkerKey = marker.dataset.markerKey
    selectedMarkerLayer = marker.parentElement.id
    marker.classList.add('selected')
  })

  mapViewport.addEventListener('wheel', (event) => {
    event.preventDefault()
    zoomAt(view.scale * (event.deltaY < 0 ? 1.16 : 0.86), event.clientX, event.clientY)
  }, { passive: false })

  mapViewport.addEventListener('pointerdown', (event) => {
    if (event.button !== 0) return
    drag = { pointer: event.pointerId, x: event.clientX, y: event.clientY, viewX: view.x, viewY: view.y }
    mapViewport.setPointerCapture(event.pointerId)
    mapViewport.classList.add('dragging')
  })
  mapViewport.addEventListener('pointermove', (event) => {
    updateCoordinates(event)
    if (!drag || drag.pointer !== event.pointerId) return
    view.x = drag.viewX + event.clientX - drag.x
    view.y = drag.viewY + event.clientY - drag.y
    clampView()
    applyView()
  })
  mapViewport.addEventListener('pointerup', endDrag)
  mapViewport.addEventListener('pointercancel', endDrag)

  function endDrag(event) {
    if (!drag || drag.pointer !== event.pointerId) return
    drag = null
    mapViewport.classList.remove('dragging')
  }

  function updateCoordinates(event) {
    if (!activeLayer) return
    const rect = mapViewport.getBoundingClientRect()
    const sceneX = (event.clientX - rect.left - view.x) / view.scale
    const sceneY = (event.clientY - rect.top - view.y) / view.scale
    const [maxX, maxY, minX, minY] = activeLayer.bounds
    const worldY = minY + (sceneX / SCENE_SIZE) * (maxY - minY)
    const worldX = maxX - (sceneY / SCENE_SIZE) * (maxX - minX)
    cursorCoordinates.textContent = `X ${Math.round(worldX)} · Y ${Math.round(worldY)}`
  }

  legend.addEventListener('change', (event) => {
    if (!event.target.matches('input[type="checkbox"]')) return
    if (event.target.checked) enabledKinds.add(event.target.value)
    else enabledKinds.delete(event.target.value)
    renderMarkers()
  })
  searchInput.addEventListener('input', renderMarkers)
  levelFilter.addEventListener('change', renderMarkers)

  function setFiltersOpen(open) {
    filterPanel.hidden = !open
    toggleFilters.setAttribute('aria-expanded', String(open))
  }

  toggleFilters.addEventListener('click', () => setFiltersOpen(filterPanel.hidden))
  closeFilters.addEventListener('click', () => setFiltersOpen(false))
  document.querySelector('#zoomIn').addEventListener('click', () => {
    const rect = mapViewport.getBoundingClientRect()
    zoomAt(view.scale * 1.35, rect.left + rect.width / 2, rect.top + rect.height / 2)
  })
  document.querySelector('#zoomOut').addEventListener('click', () => {
    const rect = mapViewport.getBoundingClientRect()
    zoomAt(view.scale / 1.35, rect.left + rect.width / 2, rect.top + rect.height / 2)
  })
  document.querySelector('#fitMap').addEventListener('click', resetView)
  new ResizeObserver(resetView).observe(mapViewport)

  function updateAge() {
    if (!playerState?.lastSuccessAt) {
      updatedText.textContent = 'Waiting for data'
      return
    }
    const seconds = Math.max(0, Math.round((Date.now() - new Date(playerState.lastSuccessAt).getTime()) / 1000))
    updatedText.textContent = seconds < 2 ? 'Updated now' : `Updated ${seconds}s ago`
  }

  window.setInterval(updateAge, 1000)
  boot()
})()
