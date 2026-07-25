export interface User {
  id: number
  username: string
  display_name?: string
  description?: string
  image_url?: string
  public: boolean
  is_admin: boolean
  created_at: string
}

export interface Artist {
  id: number
  name: string
  description?: string
  image_url?: string
  pinned: boolean
}

export interface Album {
  id: number
  name: string
  description?: string
  image_url?: string
  pinned: boolean
}

export interface Song {
  id: number
  name: string
  duration_ms?: number
  image_url?: string
  pinned: boolean
  artist_id?: number
  artist_name?: string
  album_id?: number
  album_name?: string
}

export interface Track extends Song {
  track_number?: number
}

export interface EntitySummary {
  plays: number
  unique_listeners?: number
  minutes_listened: number
  first_listened_at?: string
}

export interface ArtistDetail extends Artist {
  albums: Album[]
  songs: Song[]
}

export interface AlbumDetail extends Album {
  artists: Artist[]
  tracks: Track[]
}

export interface SongDetail extends Song {
  artists: Artist[]
  album?: Album
  track_number?: number
}

export interface Alias {
  id: number
  alias: string
}

export interface ApiKey {
  id: number
  name: string
  created_at: string
  last_used_at?: string
}

export interface ApiKeyCreated extends ApiKey {
  key: string
}

export interface Invite {
  id: number
  code: string
  name: string
  max_uses: number | null
  uses_count: number
  created_at: string
}

export type ImportStatus = 'queued' | 'running' | 'paused' | 'completed' | 'failed' | 'cancelled'
export type ImportService = 'spotify' | 'ytmusic' | 'lastfm' | 'listenbrainz' | 'maloja' | 'canto_export' | 'koito'

export interface ImportJob {
  id: number
  batch_id: string
  filename: string
  service: ImportService
  status: ImportStatus
  total_items: number
  processed_items: number
  imported_items: number
  skipped_items: number
  failed_items: number
  error_message?: string
  created_at: string
  started_at?: string
  finished_at?: string
}

export interface ForwardRule {
  ingester: string
  type: string
  url: string
  token: string
}

export interface Settings {
  link_processors: string[]
  fallback_processors: string[]
  fuzzy_matchers: string[]
  ingesters: string[]
  forwards: ForwardRule[]
}

export interface RegistryProcessor {
  id: string
  can_detect: boolean
  can_lookup: boolean
  available: boolean
}

export interface RegistryMatcher {
  id: string
  available: boolean
}

export interface RegistryIngester {
  id: string
  label: string
  api_path: string
}

export interface SettingsRegistry {
  processors: RegistryProcessor[]
  matchers: RegistryMatcher[]
  ingesters: RegistryIngester[]
}

// A listen's or now-playing entry's attributed user; absent entirely when private.
export interface Listener {
  id?: number
  username?: string
  display_name?: string
  image_url?: string
}

// One listen of a catalog entity by any user (GET /artists|albums|songs/{id}/listens).
export interface EntityListen {
  listened_at: string
  user?: Listener
}

export interface EntityListensPage {
  listens: EntityListen[]
  total: number
  page: number
  per_page: number
}

// One user currently listening to a catalog entity.
export interface EntityListeningNow {
  user: Listener
  started_at: string
}

// One raw listen within a stats scope (GET /stats/{scope}/listens); user is set for global scope only.
export interface StatsListen {
  id: number
  song: Song
  listened_at: string
  duration_played_ms?: number
  user?: Listener
}

export interface StatsListensPage {
  listens: StatsListen[]
  total: number
  page: number
  per_page: number
}

// One now-playing track within a stats scope; user is set for global scope only.
export interface StatsNowPlayingEntry {
  song: Song
  started_at: string
  duration_ms?: number
  user?: Listener
}

export interface StatsSummary {
  listen_count: number
  unique_tracks: number
  unique_albums: number
  unique_artists: number
  minutes_listened: number
  days_active: number
  longest_streak: number
  current_streak: number
  avg_daily_plays: number
  avg_session_length_ms: number
  tracks_per_artist: number
  albums_per_artist: number
}

export type TopKind = 'artists' | 'albums' | 'tracks'

export interface TopEntry {
  id: number
  name: string
  artist_id?: number
  artist_name?: string
  album_id?: number
  album_name?: string
  image_url?: string
  listen_count: number
  minutes_listened: number
  blacklisted: boolean
}

export interface RewindEntry extends TopEntry {
  delta: number
}

export interface ActivityBucket {
  bucket: string
  listen_count: number
  minutes_listened: number
}

export interface ActivityResult {
  buckets: ActivityBucket[]
  longest_streak: number
  current_streak: number
}

export interface ClockCell {
  day_of_week: number
  hour: number
  listen_count: number
}

export interface SourceEntry {
  source_type: string
  listen_count: number
}

export interface DiscoveryBucket {
  bucket: string
  total: number
  discoveries: number
  discovery_rate: number
}

export interface InterestBucket {
  bucket: string
  listen_count: number
}

export interface RewindBundle {
  top_artists: RewindEntry[]
  top_albums: RewindEntry[]
  top_tracks: RewindEntry[]
  minutes_listened: number
  plays: number
  unique_tracks: number
  unique_albums: number
  unique_artists: number
  avg_daily_plays: number
  new_tracks: number
  new_albums: number
  new_artists: number
  top_day: string | null
  longest_streak: number
}

export interface OwnListen {
  listen_id: number
  listened_at: string
  song: Song
}

export type SearchType = 'artist' | 'album' | 'song' | 'user' | 'own_listens'

export interface SearchResult {
  type: SearchType
  artist?: Artist
  album?: Album
  song?: Song
  user?: User
  own_listen?: OwnListen
}

// {title, detail} error envelope Canto's fail() writes on every non-2xx response.
export interface ApiErrorBody {
  title: string
  detail: string
}
