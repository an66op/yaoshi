export type RoomLogoPreset = Readonly<{
  id: string
  label: string
  path: string
}>

export const roomLogoPresets = [
  { id: 'wangzhe-classic', label: '王者经典', path: '/images/wangzhe-header-logo.png' },
  { id: 'crown-crystal', label: '冰晶王冠', path: '/images/room-logos/crown-crystal.webp' },
  { id: 'crown-shield', label: '王冠盾徽', path: '/images/room-logos/crown-shield.webp' },
  { id: 'crown-laurel', label: '桂冠王冠', path: '/images/room-logos/crown-laurel.webp' },
] as const satisfies readonly RoomLogoPreset[]
