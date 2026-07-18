(() => {
  'use strict'

  const MAP_PIXEL_SIZE = 8192
  const SCENE_SIZE = MAP_PIXEL_SIZE / Math.max(1, window.devicePixelRatio || 1)
  const MAX_ZOOM_RATIO = 96
  const allKinds = ['players', 'bases', 'workers', 'companions', 'wild-pals', 'npcs']
  const enabledKinds = new Set(allKinds)

  const siteTitle = document.querySelector('#siteTitle')
  const demoBadge = document.querySelector('#demoBadge')
  const serverDescription = document.querySelector('#serverDescription')
  const statusDot = document.querySelector('#statusDot')
  const statusText = document.querySelector('#statusText')
  const metricsSummary = document.querySelector('#metricsSummary')
  const serverDetailsButton = document.querySelector('#serverDetailsButton')
  const updatedText = document.querySelector('#updatedText')
  const toggleFilters = document.querySelector('#toggleFilters')
  const closeFilters = document.querySelector('#closeFilters')
  const layerTabs = document.querySelector('#layerTabs')
  const filterPanel = document.querySelector('#filterPanel')
  const searchInput = document.querySelector('#searchInput')
  const mapExplorer = document.querySelector('#mapExplorer')
  const objectNotice = document.querySelector('#objectNotice')
  const mapViewport = document.querySelector('#mapViewport')
  const mapScene = document.querySelector('#mapScene')
  const mapImage = document.querySelector('#mapImage')
  const markerLayer = document.querySelector('#markerLayer')
  const objectMarkerLayer = document.querySelector('#objectMarkerLayer')
  const playerMarkerLayer = document.querySelector('#playerMarkerLayer')
  const mapNotice = document.querySelector('#mapNotice')
  const cursorCoordinates = document.querySelector('#cursorCoordinates')
  const detailsDialog = document.querySelector('#detailsDialog')
  const closeDetails = document.querySelector('#closeDetails')
  const detailsKind = document.querySelector('#detailsKind')
  const detailsTitle = document.querySelector('#detailsTitle')
  const detailsBody = document.querySelector('#detailsBody')

  mapScene.style.setProperty('--scene-size', `${SCENE_SIZE}px`)

  let config = null
  let playerState = null
  let objectState = emptyObjectState(false)
  let activeLayer = null
  let selectedMarkerKey = null
  let selectedMarkerLayer = null
  let detailsReturnFocus = null
  const hiddenItemKeys = new Set()
  const expandedGuildIDs = new Set()
  const expandedBaseIDs = new Set()
  let view = { scale: 1, x: 0, y: 0 }
  let drag = null
  const markerItems = new WeakMap()
  const explorerItems = new Map()

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
      demoBadge.hidden = !config.demoMode
      objectState = emptyObjectState(config.worldDataEnabled)
      activeLayer = config.layers[0]
      buildLayerTabs()
      resetView()
      renderObjectAvailability()
      renderExplorer()
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
      renderExplorer()
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
    const metrics = playerState.metricsAvailable ? playerState.metrics : null
    const currentPlayers = metrics?.currentPlayers ?? playerCount
    const playerStatus = metrics
      ? `${currentPlayers} / ${metrics.maxPlayers} players`
      : `${playerCount} player${playerCount === 1 ? '' : 's'} online`
    if (playerState.connected && !playerState.stale) setStatus('live', playerStatus)
    else if (playerState.stale) setStatus('stale', `${currentPlayers}${metrics ? ` / ${metrics.maxPlayers}` : ''} last known`)
    else setStatus('offline', 'Server unavailable')
    metricsSummary.hidden = !metrics
    metricsSummary.textContent = metrics ? `· ${metrics.serverFps} FPS · Up ${formatUptime(metrics.uptimeSeconds)} · Day ${metrics.days}` : ''
    renderExplorer()
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

  function searchQuery() {
    return searchInput.value.trim().toLowerCase()
  }

  function assignedBase(item) {
    if (!item.baseId) return null
    return objectItems().find((candidate) => candidate.kind === 'bases' && candidate.baseId === item.baseId) || null
  }

  function matchesSearch(item) {
    const query = searchQuery()
    if (!query) return true
    const baseName = item.kind === 'workers' ? assignedBase(item)?.name || '' : ''
    return `${item.name} ${item.detail || ''} ${item.level || ''} ${baseName}`.toLowerCase().includes(query)
  }

  function filteredItems(items) {
    return items.filter((item) => {
      if (item.map !== activeLayer.id || !enabledKinds.has(item.kind)) return false
      if (hiddenItemKeys.has(itemKey(item))) return false
      return matchesSearch(item)
    })
  }

  function renderObjectAvailability() {
    const enabled = config?.worldDataEnabled && objectState.enabled

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

  function createElement(tagName, className, text) {
    const node = document.createElement(tagName)
    if (className) node.className = className
    if (text !== undefined) node.textContent = text
    return node
  }

  function groupKinds(group) {
    return group === 'bases' ? ['bases', 'workers'] : [group]
  }

  function itemKey(item) {
    return JSON.stringify([item.kind, item.map, item.name, item.detail || '', item.x, item.y])
  }

  function itemIsVisible(item) {
    return enabledKinds.has(item.kind) && !hiddenItemKeys.has(itemKey(item))
  }

  function createVisibilityToggle(items, visibilityID, label) {
    const checkbox = document.createElement('input')
    checkbox.type = 'checkbox'
    checkbox.className = 'item-visibility'
    checkbox.dataset.visibilityId = visibilityID
    checkbox.dataset.visibilityKeys = JSON.stringify(items.map(itemKey))
    const visibleCount = items.filter(itemIsVisible).length
    checkbox.checked = items.length > 0 && visibleCount === items.length
    checkbox.indeterminate = visibleCount > 0 && visibleCount < items.length
    checkbox.disabled = items.length === 0 || !items.some((item) => enabledKinds.has(item.kind))
    checkbox.setAttribute('aria-label', label)
    return checkbox
  }

  function createCategory(group, title, items, count) {
    const section = createElement('section', 'explorer-section')
    section.dataset.group = group
    const header = createElement('label', 'explorer-category')
    const checkbox = document.createElement('input')
    checkbox.type = 'checkbox'
    checkbox.className = 'category-toggle'
    checkbox.dataset.group = group
    checkbox.dataset.kinds = groupKinds(group).join(',')
    const enabledCount = groupKinds(group).filter((kind) => enabledKinds.has(kind)).length
    const visibleCount = items.filter(itemIsVisible).length
    checkbox.checked = items.length > 0 && enabledCount === groupKinds(group).length && visibleCount === items.length
    checkbox.indeterminate = !checkbox.checked && visibleCount > 0
    checkbox.disabled = items.length === 0
    const symbol = createElement('span', `explorer-symbol ${group}`)
    symbol.setAttribute('aria-hidden', 'true')
    const name = createElement('strong', '', `${title} (${count})`)
    header.append(checkbox, symbol, name)
    section.append(header)
    section.classList.toggle('category-hidden', enabledCount === 0)
    return section
  }

  function createExplorerItem(item, meta, className = '', displayName = item.name) {
    const key = itemKey(item)
    explorerItems.set(key, item)
    const button = createElement('button', `explorer-item ${className}`.trim())
    button.type = 'button'
    button.dataset.itemKey = key
    button.setAttribute('aria-label', `View ${markerText(item)}`)
    const symbol = createElement('span', `explorer-symbol ${item.kind}`)
    symbol.setAttribute('aria-hidden', 'true')
    const name = createElement('span', 'explorer-item-name', displayName)
    button.append(symbol, name)
    if (meta) button.append(createElement('span', 'explorer-item-meta', meta))
    if (item.detail) button.title = item.detail
    return button
  }

  function createObjectRow(item, meta, className = '', displayName = item.name) {
    const row = createElement('div', `explorer-object-row ${className ? `${className}-row` : ''}`.trim())
    row.append(
      createVisibilityToggle([item], `item:${itemKey(item)}`, `Show ${markerText(item)}`),
      createExplorerItem(item, meta, className, displayName)
    )
    return row
  }

  function itemMeta(item) {
    if (item.level > 0) return `Lv ${item.level}`
    if (item.kind === 'npcs') return item.detail
    return ''
  }

  function emptyCategoryMessage(group, currentCount) {
    const noun = {
      players: 'players',
      bases: 'bases',
      companions: 'companion Pals',
      'wild-pals': 'wild Pals',
      npcs: 'NPCs'
    }[group]
    if (currentCount > 0 && searchQuery()) return `No ${noun} match “${searchInput.value.trim()}”.`
    const existsElsewhere = allItems().some((item) => item.kind === group && item.map !== activeLayer?.id)
    if (existsElsewhere) return `No ${noun} on this map.`
    return {
      players: 'No players online.',
      bases: 'No bases are currently reported.',
      companions: 'No companion Pals are currently reported.',
      'wild-pals': 'No wild Pals are currently loaded.',
      npcs: 'No NPCs are currently loaded.'
    }[group]
  }

  function appendItemCategory(fragment, group, title, items) {
    const sorted = items.slice().sort((left, right) => left.name.localeCompare(right.name))
    const matching = sorted.filter(matchesSearch)
    const section = createCategory(group, title, sorted, sorted.length)
    const list = createElement('div', 'explorer-items')
    if (matching.length === 0) {
      list.append(createElement('p', 'explorer-empty', emptyCategoryMessage(group, sorted.length)))
    } else {
      for (const item of matching) list.append(createObjectRow(item, itemMeta(item)))
    }
    section.append(list)
    fragment.append(section)
  }

  function appendBaseCategory(fragment, bases, workers) {
    const sortedBases = bases.slice().sort((left, right) => left.name.localeCompare(right.name) || left.x - right.x || left.y - right.y)
    const section = createCategory('bases', 'Bases', bases.concat(workers), sortedBases.length)
    const list = createElement('div', 'explorer-items base-tree')
    const guilds = new Map()
    for (const base of sortedBases) {
      const guildID = base.guildKey || `base:${base.baseId}`
      if (!guilds.has(guildID)) guilds.set(guildID, { id: guildID, name: base.name, bases: [] })
      guilds.get(guildID).bases.push(base)
    }
    const guildEntries = Array.from(guilds.values()).sort((left, right) => left.name.localeCompare(right.name))
    const guildNameCounts = new Map()
    const guildNameOccurrences = new Map()
    for (const guild of guildEntries) guildNameCounts.set(guild.name, (guildNameCounts.get(guild.name) || 0) + 1)
    let matchingGuilds = 0
    for (const guild of guildEntries) {
      const occurrence = (guildNameOccurrences.get(guild.name) || 0) + 1
      guildNameOccurrences.set(guild.name, occurrence)
      const displayGuildName = guildNameCounts.get(guild.name) > 1 ? `${guild.name} #${occurrence}` : guild.name
      const baseEntries = guild.bases.map((base, index) => {
        const baseWorkers = workers
          .filter((worker) => worker.baseId === base.baseId)
          .sort((left, right) => left.name.localeCompare(right.name))
        return { base, baseWorkers, index, matchingWorkers: baseWorkers.filter(matchesSearch) }
      }).filter(({ base, matchingWorkers }) => matchesSearch(base) || matchingWorkers.length > 0)
      if (baseEntries.length === 0) continue
      matchingGuilds++

      const guildItems = guild.bases.flatMap((base) => [base].concat(workers.filter((worker) => worker.baseId === base.baseId)))
      const guildBranch = createElement('div', 'guild-branch')
      const guildRow = createElement('div', 'guild-row')
      const guildExpanded = expandedGuildIDs.has(guild.id) || Boolean(searchQuery())
      const guildExpand = createElement('button', 'branch-toggle', '›')
      guildExpand.type = 'button'
      guildExpand.dataset.branchKind = 'guild'
      guildExpand.dataset.branchId = guild.id
      guildExpand.dataset.branchName = displayGuildName
      guildExpand.setAttribute('aria-label', `${guildExpanded ? 'Collapse' : 'Expand'} ${displayGuildName}`)
      guildExpand.setAttribute('aria-expanded', String(guildExpanded))
      const guildName = createElement('span', 'guild-name', displayGuildName)
      const guildMeta = createElement('span', 'explorer-item-meta', `${guild.bases.length} base${guild.bases.length === 1 ? '' : 's'}`)
      guildRow.append(
        guildExpand,
        createVisibilityToggle(guildItems, `guild:${guild.id}`, `Show guild ${displayGuildName}`),
        guildName,
        guildMeta
      )
      guildBranch.append(guildRow)

      const guildChildren = createElement('div', 'guild-children')
      guildChildren.hidden = !guildExpanded
      for (const { base, baseWorkers, index, matchingWorkers } of baseEntries) {
        const baseBranch = createElement('div', 'base-branch')
        const row = createElement('div', 'base-row')
        const baseExpanded = expandedBaseIDs.has(base.baseId) || Boolean(searchQuery())
        const expand = createElement('button', 'branch-toggle', '›')
        expand.type = 'button'
        expand.dataset.branchKind = 'base'
        expand.dataset.branchId = base.baseId
        expand.dataset.branchName = guild.bases.length === 1 ? displayGuildName : `${displayGuildName} base ${index + 1}`
        expand.setAttribute('aria-label', `${baseExpanded ? 'Collapse' : 'Expand'} ${expand.dataset.branchName}`)
        expand.setAttribute('aria-expanded', String(baseExpanded))
        const baseItems = [base].concat(baseWorkers)
        const baseLabel = guild.bases.length === 1 ? 'Base' : `Base ${index + 1}`
        row.append(
          expand,
          createVisibilityToggle(baseItems, `base:${base.baseId}`, `Show ${baseLabel} for ${displayGuildName}`),
          createExplorerItem(base, `${baseWorkers.length} Pal${baseWorkers.length === 1 ? '' : 's'}`, 'base-item', baseLabel)
        )
        baseBranch.append(row)

        const children = createElement('div', 'base-children')
        children.hidden = !baseExpanded
        const workersToShow = searchQuery() ? matchingWorkers : baseWorkers
        for (const worker of workersToShow) children.append(createObjectRow(worker, itemMeta(worker), 'worker-item'))
        baseBranch.append(children)
        guildChildren.append(baseBranch)
      }
      guildBranch.append(guildChildren)
      list.append(guildBranch)
    }

    const unassigned = workers.filter((worker) => !worker.baseId && matchesSearch(worker))
    for (const worker of unassigned) list.append(createObjectRow(worker, itemMeta(worker), 'worker-item orphan-worker'))
    if (matchingGuilds === 0 && unassigned.length === 0) {
      list.append(createElement('p', 'explorer-empty', emptyCategoryMessage('bases', sortedBases.length)))
    }
    section.append(list)
    fragment.append(section)
  }

  function renderExplorer() {
    const scrollTop = mapExplorer.scrollTop
    const focusedItemKey = mapExplorer.contains(document.activeElement) ? document.activeElement.closest('.explorer-item')?.dataset.itemKey : null
    const focusedBranchID = mapExplorer.contains(document.activeElement) ? document.activeElement.closest('.branch-toggle')?.dataset.branchId : null
    const focusedVisibilityID = mapExplorer.contains(document.activeElement) ? document.activeElement.closest('.item-visibility')?.dataset.visibilityId : null
    const focusedGroup = mapExplorer.contains(document.activeElement) ? document.activeElement.closest('.explorer-section')?.dataset.group : null
    explorerItems.clear()
    const fragment = document.createDocumentFragment()
    const currentItems = allItems().filter((item) => item.map === activeLayer?.id)
    appendItemCategory(fragment, 'players', 'Players', currentItems.filter((item) => item.kind === 'players'))

    const bases = currentItems.filter((item) => item.kind === 'bases')
    const workers = currentItems.filter((item) => item.kind === 'workers')
    appendBaseCategory(fragment, bases, workers)

    const optionalGroups = [
      ['companions', 'Companion Pals'],
      ['wild-pals', 'Wild Pals'],
      ['npcs', 'NPCs']
    ]
    for (const [group, title] of optionalGroups) {
      const items = currentItems.filter((item) => item.kind === group)
      appendItemCategory(fragment, group, title, items)
    }

    mapExplorer.replaceChildren(fragment)
    mapExplorer.scrollTop = scrollTop
    const focusTarget = focusedItemKey
      ? Array.from(mapExplorer.querySelectorAll('.explorer-item')).find((item) => item.dataset.itemKey === focusedItemKey)
      : focusedBranchID
        ? Array.from(mapExplorer.querySelectorAll('.branch-toggle')).find((item) => item.dataset.branchId === focusedBranchID)
        : focusedVisibilityID
          ? Array.from(mapExplorer.querySelectorAll('.item-visibility')).find((item) => item.dataset.visibilityId === focusedVisibilityID)
          : focusedGroup
            ? mapExplorer.querySelector(`[data-group="${focusedGroup}"] .category-toggle`)
            : null
    focusTarget?.focus({ preventScroll: true })
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
    const fragment = document.createDocumentFragment()

    for (const item of filteredItems(items)) {
      const position = toScene(item, activeLayer)
      if (!position) continue
      const key = itemKey(item)

      const marker = document.createElement('button')
      marker.type = 'button'
      marker.className = `map-marker ${item.kind}`
      marker.dataset.markerKey = key
      marker.dataset.itemKey = key
      marker.style.left = `${position.x}px`
      marker.style.top = `${position.y}px`
      marker.style.setProperty('--marker-scale', String(markerScale(marker)))
      marker.setAttribute('aria-label', markerText(item))
      markerItems.set(marker, item)

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
    if (item.kind === 'bases' || item.kind === 'npcs' || !item.level) return item.name
    return `${item.name} · Lv ${item.level}`
  }

  function kindLabel(kind) {
    return {
      players: 'Player',
      bases: 'Base',
      workers: 'Base worker',
      companions: 'Companion Pal',
      'wild-pals': 'Wild Pal',
      npcs: 'NPC'
    }[kind] || 'Map object'
  }

  function layerName(mapID) {
    return config?.layers.find((layer) => layer.id === mapID)?.name || mapID
  }

  function createFactList(entries) {
    const list = document.createElement('dl')
    list.className = 'details-facts'
    for (const [label, value] of entries) {
      if (value === undefined || value === null || value === '') continue
      const term = document.createElement('dt')
      term.textContent = label
      const description = document.createElement('dd')
      description.textContent = String(value)
      list.append(term, description)
    }
    return list
  }

  function showDetails(kind, title, returnFocus, render) {
    detailsKind.textContent = kind
    detailsTitle.textContent = title
    detailsBody.replaceChildren()
    detailsReturnFocus = returnFocus
    render(detailsBody)
    if (!detailsDialog.open) detailsDialog.showModal()
  }

  function itemDetails(item, marker, returnFocus) {
    const base = assignedBase(item)
    const workers = item.kind === 'bases'
      ? objectItems().filter((candidate) => candidate.kind === 'workers' && candidate.baseId === item.baseId)
      : []
    const entries = []
    if (item.level > 0) entries.push(['Level', item.level])
    if (item.detail && item.kind !== 'players') {
      entries.push([item.kind === 'npcs' ? 'Type' : 'Species', item.detail])
    }
    if (item.kind === 'workers' && base) entries.push(['Assigned base', base.name])
    if (item.kind === 'bases') entries.push(['Workers', workers.length])
    entries.push(['Region', layerName(item.map)])
    entries.push(['Coordinates', `X ${Math.round(item.x)} · Y ${Math.round(item.y)}`])

    showDetails(kindLabel(item.kind), item.name, returnFocus || {
      element: marker,
      layerID: marker.parentElement.id,
      markerKey: marker.dataset.markerKey
    }, (body) => {
      body.append(createFactList(entries))
      if (item.kind !== 'bases') return

      const section = document.createElement('section')
      section.className = 'details-section'
      const heading = document.createElement('h3')
      heading.textContent = 'Base workers'
      section.append(heading)
      if (workers.length === 0) {
        const empty = document.createElement('p')
        empty.className = 'details-empty'
        empty.textContent = 'No workers are currently reported for this base.'
        section.append(empty)
      } else {
        const roster = document.createElement('ul')
        roster.className = 'details-roster'
        workers
          .slice()
          .sort((left, right) => left.name.localeCompare(right.name))
          .forEach((worker) => {
            const row = document.createElement('li')
            const name = document.createElement('span')
            name.textContent = worker.level > 0 ? `${worker.name} · Lv ${worker.level}` : worker.name
            const species = document.createElement('span')
            species.textContent = worker.detail || 'Pal'
            row.append(name, species)
            roster.append(row)
          })
        section.append(roster)
      }
      body.append(section)
    })
  }

  function formatUptime(seconds) {
    const days = Math.floor(seconds / 86400)
    const hours = Math.floor((seconds % 86400) / 3600)
    const minutes = Math.floor((seconds % 3600) / 60)
    const parts = []
    if (days) parts.push(`${days}d`)
    if (days || hours) parts.push(`${hours}h`)
    parts.push(`${minutes}m`)
    return parts.join(' ')
  }

  function serverDetails() {
    const server = playerState?.server || {}
    const metrics = playerState?.metricsAvailable ? playerState.metrics : null
    showDetails('Server', server.name || 'Palworld server', { element: serverDetailsButton }, (body) => {
      if (server.description) {
        const description = document.createElement('p')
        description.className = 'details-description'
        description.textContent = server.description
        body.append(description)
      }
      const entries = []
      if (metrics) {
        entries.push(
          ['Players', `${metrics.currentPlayers} / ${metrics.maxPlayers}`],
          ['Server FPS', metrics.serverFps],
          ['Average FPS', metrics.averageFps.toFixed(1)],
          ['Frame time', `${metrics.serverFrameTime.toFixed(2)} ms`],
          ['Uptime', formatUptime(metrics.uptimeSeconds)],
          ['Base camps', metrics.baseCount],
          ['In-game day', metrics.days]
        )
      } else {
        entries.push(['Metrics', 'Temporarily unavailable'])
      }
      if (server.version) entries.push(['Version', server.version])
      if (playerState?.metricsUpdatedAt) {
        entries.push(['Metrics updated', new Date(playerState.metricsUpdatedAt).toLocaleString()])
      }
      body.append(createFactList(entries))
    })
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
        renderExplorer()
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
    for (const marker of markerLayer.querySelectorAll('.map-marker')) {
      marker.style.setProperty('--marker-scale', String(markerScale(marker)))
    }
  }

  function markerScale(marker) {
    const zoomRatio = Math.max(1, view.scale / fitScale())
    const screenScale = marker?.classList.contains('workers') ? 1 : Math.min(2, Math.sqrt(zoomRatio))
    return screenScale / view.scale
  }

  function zoomAt(nextScale, clientX, clientY) {
    const rect = mapViewport.getBoundingClientRect()
    const minimum = fitScale()
    nextScale = Math.min(minimum * MAX_ZOOM_RATIO, Math.max(minimum, nextScale))
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

  function findMarkerForItem(key) {
    return Array.from(markerLayer.querySelectorAll('.map-marker')).find((marker) => marker.dataset.itemKey === key) || null
  }

  function focusExplorerItem(button) {
    const item = explorerItems.get(button.dataset.itemKey)
    if (!item) return
    for (const kind of groupKinds(item.kind === 'workers' ? 'bases' : item.kind)) enabledKinds.add(kind)
    hiddenItemKeys.delete(button.dataset.itemKey)
    renderExplorer()
    const refreshedButton = Array.from(mapExplorer.querySelectorAll('.explorer-item')).find((candidate) => candidate.dataset.itemKey === button.dataset.itemKey) || button
    renderMarkers()
    const marker = findMarkerForItem(button.dataset.itemKey)
    if (!marker) return

    const position = toScene(item, activeLayer)
    const viewport = mapViewport.getBoundingClientRect()
    const targetZoom = fitScale() * (item.kind === 'workers' ? 24 : 8)
    view.scale = Math.min(fitScale() * MAX_ZOOM_RATIO, Math.max(view.scale, targetZoom))
    view.x = viewport.width / 2 - position.x * view.scale
    view.y = viewport.height / 2 - position.y * view.scale
    clampView()
    applyView()

    clearSelection()
    selectedMarkerKey = marker.dataset.markerKey
    selectedMarkerLayer = marker.parentElement.id
    marker.classList.add('selected')
    itemDetails(item, marker, { element: refreshedButton, explorerKey: button.dataset.itemKey })
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
    const item = markerItems.get(marker)
    if (item) itemDetails(item, marker)
  })

  mapViewport.addEventListener('wheel', (event) => {
    event.preventDefault()
    zoomAt(view.scale * (event.deltaY < 0 ? 1.16 : 0.86), event.clientX, event.clientY)
  }, { passive: false })

  mapViewport.addEventListener('pointerdown', (event) => {
    if (event.button !== 0 || event.target.closest('.filter-panel, .filter-toggle, .map-controls')) return
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

  mapExplorer.addEventListener('change', (event) => {
    if (event.target.matches('.category-toggle')) {
      const categoryKinds = event.target.dataset.kinds.split(',')
      for (const kind of categoryKinds) {
        if (event.target.checked) enabledKinds.add(kind)
        else enabledKinds.delete(kind)
      }
      if (event.target.checked) {
        for (const item of allItems()) {
          if (item.map === activeLayer.id && categoryKinds.includes(item.kind)) hiddenItemKeys.delete(itemKey(item))
        }
      }
      renderExplorer()
      renderMarkers()
      return
    }
    if (event.target.matches('.item-visibility')) {
      for (const key of JSON.parse(event.target.dataset.visibilityKeys)) {
        if (event.target.checked) hiddenItemKeys.delete(key)
        else hiddenItemKeys.add(key)
      }
      renderExplorer()
      renderMarkers()
    }
  })
  mapExplorer.addEventListener('click', (event) => {
    const branchToggle = event.target.closest('.branch-toggle')
    if (branchToggle) {
      const expanded = branchToggle.getAttribute('aria-expanded') !== 'true'
      branchToggle.setAttribute('aria-expanded', String(expanded))
      branchToggle.setAttribute('aria-label', `${expanded ? 'Collapse' : 'Expand'} ${branchToggle.dataset.branchName}`)
      const isGuild = branchToggle.dataset.branchKind === 'guild'
      const branch = branchToggle.closest(isGuild ? '.guild-branch' : '.base-branch')
      branch.querySelector(isGuild ? ':scope > .guild-children' : ':scope > .base-children').hidden = !expanded
      const expandedIDs = isGuild ? expandedGuildIDs : expandedBaseIDs
      if (expanded) expandedIDs.add(branchToggle.dataset.branchId)
      else expandedIDs.delete(branchToggle.dataset.branchId)
      return
    }
    const itemButton = event.target.closest('.explorer-item')
    if (itemButton) focusExplorerItem(itemButton)
  })
  searchInput.addEventListener('input', () => {
    renderExplorer()
    renderMarkers()
  })

  function setFiltersOpen(open) {
    filterPanel.hidden = !open
    toggleFilters.hidden = open
    toggleFilters.setAttribute('aria-expanded', String(open))
  }

  toggleFilters.addEventListener('click', () => setFiltersOpen(true))
  closeFilters.addEventListener('click', () => setFiltersOpen(false))
  serverDetailsButton.addEventListener('click', serverDetails)
  closeDetails.addEventListener('click', () => detailsDialog.close())
  detailsDialog.addEventListener('click', (event) => {
    if (event.target !== detailsDialog) return
    const bounds = detailsDialog.getBoundingClientRect()
    const inside = event.clientX >= bounds.left && event.clientX <= bounds.right && event.clientY >= bounds.top && event.clientY <= bounds.bottom
    if (!inside) detailsDialog.close()
  })
  detailsDialog.addEventListener('close', () => {
    const target = detailsReturnFocus?.element?.isConnected
      ? detailsReturnFocus.element
      : detailsReturnFocus?.explorerKey
        ? Array.from(mapExplorer.querySelectorAll('.explorer-item')).find((item) => item.dataset.itemKey === detailsReturnFocus.explorerKey)
        : findMarker(detailsReturnFocus?.layerID, detailsReturnFocus?.markerKey)
    clearSelection()
    detailsReturnFocus = null
    target?.focus({ preventScroll: true })
  })
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
