import type { MapItem } from '../types'

export interface GuildDetails {
  id: string
  name: string
  onlineMembers: MapItem[]
  bases: MapItem[]
  pals: MapItem[]
}

export function guildIdForBase(base: MapItem) {
  return base.guildKey || `base:${base.id}`
}

function compareItems(left: MapItem, right: MapItem) {
  return (
    left.name.localeCompare(right.name) || (left.level || 0) - (right.level || 0) || left.id.localeCompare(right.id)
  )
}

export function buildGuildDetails(id: string, items: MapItem[]): GuildDetails {
  const playersById = new Map<string, MapItem>()
  const basesById = new Map<string, MapItem>()
  const guildByOwnerId = new Map<string, string>()

  for (const item of items) {
    if (item.kind === 'players') playersById.set(item.id, item)
    if (item.ownerId && item.guildKey) guildByOwnerId.set(item.ownerId, item.guildKey)
    if (item.kind !== 'bases') continue
    basesById.set(item.id, item)
    if (item.baseId) basesById.set(item.baseId, item)
  }

  const playerGuild = (player: MapItem) => player.guildKey || guildByOwnerId.get(player.id)
  const guildForPal = (pal: MapItem) => {
    if (pal.kind === 'workers' && pal.baseId) {
      const base = basesById.get(pal.baseId)
      if (base) return guildIdForBase(base)
    }
    if (pal.guildKey) return pal.guildKey
    return pal.ownerId ? guildByOwnerId.get(pal.ownerId) || playersById.get(pal.ownerId)?.guildKey : undefined
  }

  const onlineMembers = items.filter((item) => item.kind === 'players' && playerGuild(item) === id).sort(compareItems)
  const bases = items
    .filter((item) => item.kind === 'bases' && guildIdForBase(item) === id)
    .sort((left, right) => left.x - right.x || left.y - right.y || left.id.localeCompare(right.id))
  const pals = items
    .filter((item) => (item.kind === 'workers' || item.kind === 'companions') && guildForPal(item) === id)
    .sort(compareItems)
  const namedBase = bases.find((base) => base.name.trim().toLowerCase() !== 'palbox')
  const name = onlineMembers.find((member) => member.guildName)?.guildName || namedBase?.name || 'Unnamed guild'

  return { id, name, onlineMembers, bases, pals }
}
