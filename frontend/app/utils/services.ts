export interface ServiceMeta {
  label: string
  icon: string
}

const services: Record<string, ServiceMeta> = {
  spotify: { label: 'Spotify', icon: 'fa6-brands:spotify' },
  ytmusic: { label: 'YouTube Music', icon: 'fa6-brands:youtube' },
  lastfm: { label: 'Last.fm', icon: 'fa6-brands:lastfm' },
  deezer: { label: 'Deezer', icon: 'fa6-brands:deezer' },
  bandcamp: { label: 'Bandcamp', icon: 'fa6-brands:bandcamp' },
  musicbrainz: { label: 'MusicBrainz', icon: 'fa6-solid:compact-disc' },
  listenbrainz: { label: 'ListenBrainz', icon: 'fa6-solid:headphones' },
  subsonic: { label: 'Subsonic', icon: 'fa6-solid:server' },
  maloja: { label: 'Maloja', icon: 'fa6-solid:chart-simple' },
  koito: { label: 'Koito', icon: 'fa6-solid:record-vinyl' },
  canto_export: { label: 'Canto export', icon: 'fa6-solid:file-export' },
  exact: { label: 'Exact match', icon: 'fa6-solid:equals' },
  meilisearch: { label: 'Meilisearch', icon: 'fa6-solid:magnifying-glass' },
  unknown: { label: 'Unknown', icon: 'fa6-solid:circle-question' },
}

// serviceMeta looks up id's display label/icon, falling back to id itself for anything unmapped.
export function serviceMeta(id: string): ServiceMeta {
  return services[id] ?? { label: id, icon: 'fa6-solid:music' }
}
