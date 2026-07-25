# Correlation rework

Plan for replacing name normalization as the cross-source bridge in `internal/correlate`.

## Problem

Normalization is currently load-bearing for entity identity. `NormalizeName` lowercases, folds
diacritics, strips punctuation, collapses whitespace. That cannot bridge:

- YouTube Music's `Original Title - Localized Title` song titles.
- `ヨルシカ` (YTM) against `Yorushika` (Spotify). Different scripts, zero shared tokens.
- `Song (feat. X)` (Spotify) against `Song` with X in the artist list (YTM).
- Version suffixes: `- 2011 Remaster`, `- Live`, `(Deluxe Edition)`, `- Radio Edit`.

Normalization should be a fuzzy-matching input only. It is currently three other things as well:

- An identity comparison. `FindSongByExactName` and siblings match `name = $1 OR name_normalized = $1`.
- An advisory lock key. `AdvisoryLockEntityName` hashes `name_normalized`.
- A config-switchable behaviour. `ProcessorSettings.Normalize` flips `matchName` between raw and
  normalized, so a flag changes what counts as the same entity.

## Bugs found along the way

These are independent of the rework and some are corrupting data today.

**Source IDs are not scoped by entity type.** `GetSourceEntityID` filters on `source_type` and
`extracted_id` only, and the unique index in `00001_init.sql` is `(source_type, extracted_id)`
globally. Deezer emits bare numeric IDs for all three entity types, so Deezer artist `27`, album
`27` and track `27` collide. `ResolveArtist(deezer, "27")` can return a song ID. Last.fm uses artist
names as `extracted_id`, same hazard.

**Source conflicts are discarded.** `attachSource` swallows `pgx.ErrNoRows` from
`InsertSourceIfAbsent`. A conflict means that `(source_type, extracted_id)` already belongs to a
different entity, which is proof the two entities are the same thing. Right now it is dropped and
the loser becomes a permanent orphan duplicate.

**Meilisearch is queried with `limit 1` and a bare score threshold.** `MeilisearchMatcher.Match`
takes `hits[0]` if `RankingScore >= 0.8`. Ranking score is relevance, not similarity. A query for
`Intro` against a document named `Intro` scores 1.0, and so do the other fifty songs called `Intro`.
No runner-up comparison, no duration check, no version check.

**Artist scope is a retrieval filter.** `MeilisearchMatcher.Match` filters on `artist_ids IN [...]`.
When artist correlation forks (`ヨルシカ` as artist 5, `Yorushika` as artist 9), every song under the
losing ID is not merely ranked lower, it is absent from the candidate set. Unrecoverable at the
song level.

**Unscoped exact matches collapse distinct entities.** `FindAlbumByExactName` and
`FindSongByExactName` are `SELECT * FROM ... WHERE name = $1 LIMIT 1` with no artist scope and no
`ORDER BY`. Every `Greatest Hits` and every `Intro` resolves to a non-deterministic row.

**Matchers short-circuit.** `chainMatch` returns the first matcher's first hit, so a hit from
`exact` means `meilisearch` never runs.

## Design

### Layers

1. **Source ID.** Exact, authoritative, transactional. The steady-state path.
2. **Candidate retrieval.** Every configured matcher runs and contributes candidates.
3. **Scoring.** One scorer ranks the union and decides.

### Matchers become candidate providers

```go
type Candidate struct {
    EntityID int64
    Source   string
    Hint     float64
}

type Query struct {
    Names       []string
    ArtistIDs   []int64
    ArtistNames []string
    AlbumID     *int64
    DurationMs  *int32
}

type FuzzyMatcher interface {
    ID() string
    Candidates(ctx context.Context, entityType string, q Query) ([]Candidate, error)
}
```

Every matcher in `MatcherOrder` runs on every resolve. Candidates union by entity ID. `MatcherOrder`
becomes a set plus a retrieval-priority hint rather than a precedence chain. Registry: `exact`,
`trigram`, `meilisearch`.

Artist scope moves out of retrieval and into scoring as a hard veto on zero overlap. Same strictness,
but it can compare artist *names* rather than only IDs, so a forked artist ID no longer hides the
song. Meilisearch queries go to `limit N`.

### Romanization

Meilisearch does CJK segmentation, not transliteration. No setting makes `Yorushika` match `ヨルシカ`.
Romanization is computed in Go and indexed as a separate field.

| script | mechanism | dependency |
| --- | --- | --- |
| kana | lookup table | none |
| hangul | Revised Romanization, arithmetic on the syllable block | none |
| kanji | morphological analyzer for readings | `kagome/v2` + `kagome-dict/ipa` |
| hanzi | pinyin | `go-pinyin` |

New package `internal/correlate/romanize`, exporting `Romanize(string) string`.

**Partial romanization is worse than none.** Romanizing only the kana in `残酷な天使のテーゼ` yields
`na tenshino`, a fragment that will false-match. `Romanize` returns empty unless the entire string is
covered. Empty means the other signals carry the decision.

Script routing: any hangul means Korean. Any kana means Japanese, route to kagome. Pure Han with no
kana is ambiguous between Japanese and Chinese and defaults to Japanese. `go-pinyin` therefore only
runs on text that is explicitly Chinese by some other signal; a Chinese-heavy library would want this
default flipped.

Romanization is applied at index time and at query time.

### Columns stay separate

```
artists / albums / songs:
  name             raw, what exact matches on
  name_normalized  NormalizeName output, fuzzy input only
  name_romanized   empty unless fully romanizable
```

Not merged, for two reasons. Collapsing them loses scorer signal: a match that only appears after
romanization is a cross-script hit, weaker than two Latin strings agreeing directly, and should lean
harder on duration and artist corroboration. And it creates silent false positives: a Latin-named
band `Zankoku` becomes byte-identical to `残酷`.

Keeping them separate costs nothing. Meilisearch searches any number of `searchableAttributes` and
trigram takes a second GIN index.

### Scorer

```
score = w_name * nameSim + w_artist * artistOverlap + w_dur * durSim
```

`nameSim` is the max over (raw × raw, normalized × normalized, romanized × romanized), using
Jaro-Winkler and token-set ratio.

Vetoes:

- Duration delta over 15s.
- Zero artist overlap by ID and by name.
- Asymmetric version keyword. Substring scan for `live`, `remaster`, `acoustic`, `radio edit`,
  `instrumental`, `demo`. Present in one name and absent in the other means veto. No title splitting,
  so the `-` ambiguity never arises.

Ambiguity is priced into the score rather than gating it, so that one threshold decides the outcome:

```
gap        = best - runnerUp          runnerUp is 0 when there is only one candidate
finalScore = best - w_amb * max(0, gapFloor - gap)
```

A candidate that clears the field by `gapFloor` or more takes no penalty, so `finalScore == best` and
`autoAccept` behaves as a plain threshold. Candidates that are close to indistinguishable are
discounted in proportion to how close they are. Fifty songs named `Intro` all scoring 1.0 produce a
gap of 0 and the full deduction, dropping to a suggestion rather than binding at random.

Three bands, on `finalScore`:

| condition | action |
| --- | --- |
| `finalScore >= autoAccept` | bind, record aliases, attach source |
| `suggestMin <= finalScore < autoAccept` | create new, queue a merge suggestion |
| `finalScore < suggestMin` | create new, silently |

The third band is the anti-spam valve. Suggestions carry a unique index on the normalized
`(lo_id, hi_id)` pair with `ON CONFLICT DO NOTHING`, plus a `rejected` flag the reconciler honours, so
repeated imports cannot multiply rows and dismissals stick.

Weights and thresholds go in config, tuned against real import data.

### Title parsing: rejected

Splitting `Original - Localized` is unreliable because original titles legitimately contain `-`.
Raw titles go to Meilisearch and its tokenizer does the work: indexing `A Cruel Angel's Thesis` and
querying `残酷な天使のテーゼ - A Cruel Angel's Thesis` matches all four English tokens, with the
Japanese tokens as unmatched noise.

This is direction-sensitive. A short query against a long indexed name matches every query term and
scores high. A long query against a short indexed name leaves half the terms unmatched and scores
low, so import order affects whether a pair correlates. Mitigated by recording the incoming raw name
as an alias on accept, and indexing aliases as a searchable attribute, so an entity accumulates both
spellings and later queries match either. `entity_aliases` already exists and is unused by
correlation.

### Locking

Keep the existing advisory lock on `name_normalized`.

Blocking keys were considered and rejected. Their purpose was serializing variant spellings arriving
concurrently, but within a single import every occurrence of an entity carries the identical name
string, so racing workers hash to the same bucket and the in-lock `ExactMatcher` recheck resolves it
against Postgres. Cross-platform variants do not race because imports are serialized with a drain
barrier between them. The residue is two concurrent workers hitting different spellings of one entity
inside a single import, which is rare and which the reconciler catches.

### Meilisearch consistency

Indexing is asynchronous. `Upsert` swallows every error by design, so the index can lag or silently
miss writes.

Not made synchronous. Per-document `waitForTask` serializes 32 import workers on a single indexing
queue, and it would still not be transactional with Postgres, since a rolled-back transaction leaves
a phantom document with no rollback path.

Instead:

- **Drain barrier.** `GET /tasks?indexUids=artists,albums,songs&statuses=enqueued,processing`, polled
  until `results` is empty. `enqueued` and `processing` are the only unfinished states. Run once at
  the end of an import job, before marking it completed. One poll loop per job, not per document.
- **Serialized imports.** One bulk import job at a time. `runAsync` currently spawns per job with
  `m.sem` bounding only total workers, so jobs interleave. A job-level mutex plus the barrier means
  each import sees a fully indexed catalog from the previous one.
- **No listen queueing.** `runTimeout` is 6 hours; holding live scrobbles that long breaks
  now-playing and streak tracking, and would need a durable queue with restart survival, ordering and
  an overflow cap. A live listen arriving mid-import degrades gracefully: it may miss a fuzzy match
  and create a new entity, which the reconciler resolves.

Elasticsearch and OpenSearch were evaluated and rejected. `refresh=wait_for` gives read-your-writes
as a first-class flag, but is still not transactional with Postgres and still too slow at import
concurrency. Their ICU `Any-Latin` transform romanizes kana, hangul and hanzi, but applies Mandarin
readings to Japanese kanji, so it does not solve the hard case. Cost would be a JVM at roughly 2GB
against Meilisearch's ~100MB, a `vm.max_map_count` host sysctl, a custom image because `analysis-icu`
is a plugin rather than bundled, and a full rewrite of `internal/search/client.go`.

### Reconciler

Background worker, none exists today. Merge itself is already built and transactional, exposed as
admin-only `POST /{artists,albums,songs}/{id}/merge`, repointing sources, links, listens and
blacklist entries before deleting the loser. The reconciler reuses those `Repoint*ForMerge` queries
and only adds detection.

Auto-merges above threshold, queues the rest as suggestions. Never touches a `pinned` entity, which
means user-curated and not to be changed; those are suggested only. Also re-scans when new aliases
are learned, which no synchronous path can do.

Scope each post-import pass to the entities that import touched rather than scanning the whole
catalog.

## Constraints

- No migrations before v1.0. Schema changes are edited directly into `00001_init.sql`.
- No comments added to schema or query files.
- Go doc comments are one line, per the repo rules.
- No backfill or re-correlation of the existing catalog.

## Work order

1. `sources` unique index becomes `(entity_type, source_type, extracted_id)`; `GetSourceEntityID`
   gains an `entity_type` parameter; `InsertSourceIfAbsent` conflicts resolve the current owner and
   route a mismatch to the merge path instead of being swallowed.
2. `Candidate` and `Query` types; `FuzzyMatcher` becomes a candidate provider; existing matchers
   ported; `limit 1` becomes `limit N`; artist filter removed from retrieval.
3. `internal/correlate/romanize` with all four script tiers; `name_romanized` columns;
   `search.Document.NameRomanized`; added to `searchableAttributes`; `Reindex` populates it.
4. Scorer with vetoes and the three bands.
5. `trigram` matcher and GIN indexes on `name_normalized` and `name_romanized`.
6. Alias recording on accept; aliases indexed as a searchable attribute.
7. Merge suggestions table and reconciler.
8. Serialized import jobs and the drain barrier.
9. Strip `ProcessorsConfig.Normalize`, `ProcessorSettings.Normalize`, `refresh.Worker.normalize`, and
   every `OR name_normalized = $1` identity comparison.

Steps 1 and 3 are independent and land safely on their own.

## Open

- Alias recording on accept: unconditional, or only above a threshold higher than `autoAccept`? Bad
  aliases are sticky and poison exact lookup permanently.
- `Various Artists` and `Unknown Artist` as never-matchable sentinels?
- Album track-set overlap as a scoring signal. Strong for `Album` against `Album (Deluxe Edition)`.
- `ResolveSong` re-links artists outside the transaction after `createSongLocked` already linked them
  inside. Redundant writes on every create; skip the outer loop when `created` is true. A full
  cross-entity transaction was considered and rejected, since it would hold the artist advisory lock
  across album and song work and serialize the import pool.
