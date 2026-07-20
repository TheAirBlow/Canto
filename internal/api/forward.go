package api

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// dispatchForwards crafts and sends a copy of item to every forward rule matching ingester.
func (s *Server) dispatchForwards(rules []forwardRule, ingester, listenType string, item lbListen) {
	for _, rule := range rules {
		if rule.Ingester != ingester {
			continue
		}
		go func(rule forwardRule) {
			switch rule.Type {
			case "listenbrainz", "":
				s.forwardToListenBrainz(rule, listenType, item)
			case "lastfm":
				s.forwardToLastFM(rule, listenType, item)
			default:
				slog.Warn("unknown forward type, skipping", "type", rule.Type, "url", rule.URL)
			}
		}(rule)
	}
}

// forwardToListenBrainz crafts a ListenBrainz submit-listens request for item and sends it to rule's URL.
func (s *Server) forwardToListenBrainz(rule forwardRule, listenType string, item lbListen) {
	body, err := json.Marshal(lbSubmitRequest{ListenType: listenType, Payload: []lbListen{item}})
	if err != nil {
		slog.Warn("forward: build listenbrainz body failed", "url", rule.URL, "err", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(rule.URL, "/")+"/1/submit-listens", bytes.NewReader(body))
	if err != nil {
		slog.Warn("forward: build listenbrainz request failed", "url", rule.URL, "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Token "+rule.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("forward: listenbrainz request failed", "url", rule.URL, "err", err)
		return
	}
	resp.Body.Close()
}

// forwardToLastFM crafts a Last.fm scrobble API call for item and sends it.
func (s *Server) forwardToLastFM(rule forwardRule, listenType string, item lbListen) {
	if s.providers.LastFMAPIKey == "" || s.providers.LastFMAPISecret == "" {
		slog.Warn("forward: lastfm app credentials not configured, skipping", "url", rule.URL)
		return
	}

	method := "track.scrobble"
	if listenType == "playing_now" {
		method = "track.updateNowPlaying"
	}

	params := map[string]string{
		"method":  method,
		"api_key": s.providers.LastFMAPIKey,
		"sk":      rule.Token,
		"artist":  item.TrackMetadata.ArtistName,
		"track":   item.TrackMetadata.TrackName,
	}
	if item.TrackMetadata.ReleaseName != "" {
		params["album"] = item.TrackMetadata.ReleaseName
	}
	if method == "track.scrobble" {
		params["timestamp"] = strconv.FormatInt(item.ListenedAt, 10)
	}
	params["api_sig"] = lastFMSign(params, s.providers.LastFMAPISecret)
	params["format"] = "json"

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}

	req, err := http.NewRequest(http.MethodPost, "https://ws.audioscrobbler.com/2.0/", strings.NewReader(form.Encode()))
	if err != nil {
		slog.Warn("forward: build lastfm request failed", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("forward: lastfm request failed", "err", err)
		return
	}
	resp.Body.Close()
}

// lastFMSign computes a Last.fm API request signature.
func lastFMSign(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "format" || k == "callback" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params[k])
	}
	sb.WriteString(secret)

	sum := md5.Sum([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
