package correlate

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"Canto/internal/db"
)

// ScoringConfig weights and thresholds tune the fuzzy-match scorer against real import data.
type ScoringConfig struct {
	NameWeight      float64
	ArtistWeight    float64
	DurationWeight  float64
	TrackWeight     float64
	AmbiguityWeight float64
	GapFloor        float64
	AutoAccept      float64
	SuggestMin      float64
	DurationVetoMs  int32
}

// versionKeywords are substrings whose asymmetric presence between two names vetoes a candidate.
var versionKeywords = []string{"live", "remaster", "acoustic", "radio edit", "instrumental", "demo"}

// sentinelNames never participate in fuzzy matching; every occurrence resolves independently.
var sentinelNames = map[string]bool{"various artists": true, "unknown artist": true}

// candidateDetail is a scored candidate's full row data.
type candidateDetail struct {
	id             int64
	name           string
	nameNormalized string
	nameRomanized  string
	artistIDs      []int64
	artistNames    []string
	durationMs     *int32
	trackIDs       []int64
}

// decision is the scorer's verdict on a set of candidates.
type decision struct {
	band       decisionBand
	winnerID   int64 // valid for bandAutoAccept and bandSuggest
	finalScore float64
}

type decisionBand int

const (
	bandCreateSilent decisionBand = iota
	bandSuggest
	bandAutoAccept
)

// score picks the best candidate for a query and decides which of the three bands it falls in.
func score(cfg ScoringConfig, qRaw, qNorm, qRoman string, q Query, details []candidateDetail) decision {
	if sentinelNames[strings.ToLower(qNorm)] {
		return decision{band: bandCreateSilent}
	}

	type ranked struct {
		id  int64
		raw float64
	}
	var ranks []ranked
	for _, d := range details {
		raw, vetoed := candidateScore(cfg, qRaw, qNorm, qRoman, q, d)
		if vetoed {
			continue
		}
		ranks = append(ranks, ranked{id: d.id, raw: raw})
	}
	if len(ranks) == 0 {
		return decision{band: bandCreateSilent}
	}
	sort.Slice(ranks, func(i, j int) bool { return ranks[i].raw > ranks[j].raw })

	best := ranks[0]
	var runnerUp float64
	if len(ranks) > 1 {
		runnerUp = ranks[1].raw
	}
	gap := best.raw - runnerUp
	finalScore := best.raw - cfg.AmbiguityWeight*max(0, cfg.GapFloor-gap)

	switch {
	case finalScore >= cfg.AutoAccept:
		return decision{band: bandAutoAccept, winnerID: best.id, finalScore: finalScore}
	case finalScore >= cfg.SuggestMin:
		return decision{band: bandSuggest, winnerID: best.id, finalScore: finalScore}
	default:
		return decision{band: bandCreateSilent, finalScore: finalScore}
	}
}

// candidateScore returns d's raw (pre-ambiguity-discount) score against the query, and whether a hard veto rules it out.
func candidateScore(cfg ScoringConfig, qRaw, qNorm, qRoman string, q Query, d candidateDetail) (raw float64, vetoed bool) {
	if versionKeywordMismatch(qRaw, d.name) {
		return 0, true
	}

	durSim, durVetoed := durationSimilarity(cfg, q.DurationMs, d.durationMs)
	if durVetoed {
		return 0, true
	}

	artistSim, artistVetoed := artistOverlap(q, d)
	if artistVetoed {
		return 0, true
	}

	nameSim := nameSimilarity(qRaw, qNorm, qRoman, d)
	raw = cfg.NameWeight*nameSim + cfg.ArtistWeight*artistSim + cfg.DurationWeight*durSim
	if trackSim, ok := trackOverlap(q, d); ok {
		raw += cfg.TrackWeight * trackSim
	}
	return raw, false
}

// trackOverlap scores album track-list agreement, when both sides have a known track set.
func trackOverlap(q Query, d candidateDetail) (sim float64, ok bool) {
	if len(q.TrackIDs) == 0 || len(d.trackIDs) == 0 {
		return 0, false
	}
	return jaccardInt64(q.TrackIDs, d.trackIDs), true
}

// nameSimilarity is the max similarity across the raw/normalized/romanized pairings.
func nameSimilarity(qRaw, qNorm, qRoman string, d candidateDetail) float64 {
	best := bestSimilarity(qRaw, d.name)
	if s := bestSimilarity(qNorm, d.nameNormalized); s > best {
		best = s
	}
	if qRoman != "" && d.nameRomanized != "" {
		if s := bestSimilarity(qRoman, d.nameRomanized); s > best {
			best = s
		}
	}
	return best
}

// artistOverlap scores linked-artist agreement between the query and d, vetoing on zero overlap.
func artistOverlap(q Query, d candidateDetail) (sim float64, vetoed bool) {
	if len(q.ArtistIDs) == 0 && len(q.ArtistNames) == 0 {
		return 1, false // no artist context to compare against (e.g. resolving an artist itself)
	}
	if len(d.artistIDs) == 0 && len(d.artistNames) == 0 {
		return 1, false // candidate has no linked artists yet to compare against
	}

	idOverlap := jaccardInt64(q.ArtistIDs, d.artistIDs)
	nameOverlap := jaccardNames(q.ArtistNames, d.artistNames)
	sim = max(idOverlap, nameOverlap)
	if sim == 0 {
		return 0, true
	}
	return sim, false
}

// jaccardInt64 returns the Jaccard similarity of a and b as sets.
func jaccardInt64(a, b []int64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setA := make(map[int64]bool, len(a))
	for _, v := range a {
		setA[v] = true
	}
	setB := make(map[int64]bool, len(b))
	for _, v := range b {
		setB[v] = true
	}
	intersection := 0
	for v := range setA {
		if setB[v] {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// jaccardNames returns the Jaccard similarity of a and b's normalized names as sets.
func jaccardNames(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setA := make(map[string]bool, len(a))
	for _, v := range a {
		setA[NormalizeName(v)] = true
	}
	setB := make(map[string]bool, len(b))
	for _, v := range b {
		setB[NormalizeName(v)] = true
	}
	intersection := 0
	for v := range setA {
		if setB[v] {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// durationSimilarity scores duration agreement, vetoing a delta past cfg.DurationVetoMs.
func durationSimilarity(cfg ScoringConfig, qMs, dMs *int32) (sim float64, vetoed bool) {
	if qMs == nil || dMs == nil || *qMs <= 0 || *dMs <= 0 {
		return 1, false // no duration data on one side to compare against
	}
	delta := *qMs - *dMs
	if delta < 0 {
		delta = -delta
	}
	if delta > cfg.DurationVetoMs {
		return 0, true
	}
	return 1 - float64(delta)/float64(cfg.DurationVetoMs), false
}

// versionKeywordMismatch reports whether exactly one of a/b contains a version keyword the other lacks.
func versionKeywordMismatch(a, b string) bool {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	for _, kw := range versionKeywords {
		if strings.Contains(la, kw) != strings.Contains(lb, kw) {
			return true
		}
	}
	return false
}

// fetchCandidateDetails loads the full row data for every candidate.
func fetchCandidateDetails(ctx context.Context, queries *db.Queries, entityType string, candidates []Candidate) ([]candidateDetail, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	ids := make([]int64, len(candidates))
	for i, c := range candidates {
		ids[i] = c.EntityID
	}

	switch entityType {
	case "artist":
		rows, err := queries.GetArtistsByIDs(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("correlate: fetch artist candidates: %w", err)
		}
		details := make([]candidateDetail, len(rows))
		for i, r := range rows {
			details[i] = candidateDetail{id: r.ID, name: r.Name, nameNormalized: r.NameNormalized, nameRomanized: r.NameRomanized}
		}
		return details, nil

	case "album":
		rows, err := queries.GetAlbumsByIDs(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("correlate: fetch album candidates: %w", err)
		}
		details := make([]candidateDetail, len(rows))
		for i, r := range rows {
			artistIDs, artistNames, err := linkedArtists(ctx, queries, "album", r.ID)
			if err != nil {
				return nil, err
			}
			trackIDs, err := queries.ListSongIDsForAlbum(ctx, r.ID)
			if err != nil {
				return nil, fmt.Errorf("correlate: fetch album track ids: %w", err)
			}
			details[i] = candidateDetail{
				id: r.ID, name: r.Name, nameNormalized: r.NameNormalized, nameRomanized: r.NameRomanized,
				artistIDs: artistIDs, artistNames: artistNames, trackIDs: trackIDs,
			}
		}
		return details, nil

	case "song":
		rows, err := queries.GetSongsByIDs(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("correlate: fetch song candidates: %w", err)
		}
		details := make([]candidateDetail, len(rows))
		for i, r := range rows {
			artistIDs, artistNames, err := linkedArtists(ctx, queries, "song", r.ID)
			if err != nil {
				return nil, err
			}
			details[i] = candidateDetail{
				id: r.ID, name: r.Name, nameNormalized: r.NameNormalized, nameRomanized: r.NameRomanized,
				artistIDs: artistIDs, artistNames: artistNames, durationMs: r.DurationMs,
			}
		}
		return details, nil

	default:
		return nil, nil
	}
}

// linkedArtists returns entityID's currently-linked artist ids and names, for album/song candidates.
func linkedArtists(ctx context.Context, queries *db.Queries, entityType string, entityID int64) ([]int64, []string, error) {
	var rows []db.Artist
	var err error
	switch entityType {
	case "album":
		rows, err = queries.ListArtistsForAlbum(ctx, entityID)
	case "song":
		rows, err = queries.ListArtistsForSong(ctx, entityID)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("correlate: fetch linked artists: %w", err)
	}
	ids := make([]int64, len(rows))
	names := make([]string, len(rows))
	for i, r := range rows {
		ids[i], names[i] = r.ID, r.Name
	}
	return ids, names, nil
}
