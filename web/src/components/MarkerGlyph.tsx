import {
  IconBuildingArch,
  IconBuildingFactory2,
  IconBuildingMonument,
  IconBuildingWarehouse,
  IconCrown,
  IconHammer,
  IconHeartHandshake,
  IconMapPin,
  IconMessage2,
  IconNotebook,
  IconPaw,
  IconSkull,
  IconSparkle,
  IconTarget,
  IconTower,
  IconUser,
  IconUserPin
} from '@tabler/icons-react'
import type { ItemKind } from '../types'

const GLYPH_ICONS = {
  players: IconUser,
  bases: IconBuildingWarehouse,
  workers: IconHammer,
  companions: IconHeartHandshake,
  'wild-pals': IconPaw,
  'alpha-pals': IconCrown,
  bosses: IconSkull,
  bounties: IconTarget,
  'oil-rigs': IconBuildingFactory2,
  watchtowers: IconTower,
  waypoints: IconMapPin,
  'dungeon-entrances': IconBuildingArch,
  effigies: IconBuildingMonument,
  journals: IconNotebook,
  'ancient-shrine-pickups': IconSparkle,
  'npc-locations': IconUserPin,
  npcs: IconMessage2
} satisfies Record<ItemKind, typeof IconUser>

export function MarkerGlyph({ kind, online }: { kind: ItemKind; online?: boolean }) {
  const Glyph = GLYPH_ICONS[kind]
  const playerStatus =
    kind === 'players' ? (online === false ? 'offline' : online === true ? 'online' : undefined) : undefined
  return (
    <Glyph
      className={`marker-glyph kind-${kind}${playerStatus ? ` player-${playerStatus}` : ''}`}
      size={20}
      stroke={2.25}
      data-marker-kind={kind}
      data-player-status={playerStatus}
      aria-hidden="true"
      focusable="false"
    />
  )
}
