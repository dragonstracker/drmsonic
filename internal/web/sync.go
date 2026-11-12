package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"drmsonic/internal/config"
	"drmsonic/internal/script"
)

type pendingUpdate struct {
	kind    string
	streams []config.Stream
	summary string
}

func (s *Server) scriptsPath() string {
	if s.scriptsDir != "" {
		return s.scriptsDir
	}
	return "scripts"
}

func (s *Server) setPending(providerID string, u *pendingUpdate) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	if s.pending == nil {
		s.pending = map[string]*pendingUpdate{}
	}
	s.pending[providerID] = u
}

func (s *Server) takePending(providerID, kind string) *pendingUpdate {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	u := s.pending[providerID]
	if u == nil || u.kind != kind {
		return nil
	}
	delete(s.pending, providerID)
	return u
}

func (s *Server) scriptParams(p *config.Provider, extra ...string) []string {
	out := make([]string, 0, 8+len(extra))
	out = append(out, extra...)
	if p.Bind != "" {
		out = append(out, "bind="+p.Bind)
	}
	if p.Proxy != "" {
		out = append(out, "proxy="+p.Proxy)
	}
	if p.Doh != "" {
		out = append(out, "doh="+p.Doh)
	}
	return out
}

func (s *Server) runProviderScript(p *config.Provider, action string, extra ...string) (string, error) {
	if strings.TrimSpace(p.Script) == "" {
		return "", fmt.Errorf("provider script not set")
	}
	params := s.scriptParams(p, append([]string{"action=" + action}, extra...)...)
	out, _, err := script.Run(s.scriptsPath(), p.Script, params, 90*time.Second)
	return out, err
}

func decodeScriptStreams(raw, key string) ([]config.Stream, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("script json: %w", err)
	}
	body, ok := payload[key]
	if !ok {
		return nil, fmt.Errorf("script missing %s", key)
	}
	var rows []config.Stream
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("script %s: %w", key, err)
	}
	for i := range rows {
		if rows[i].Id == "" {
			rows[i].Id = config.Slug(rows[i].Name)
		}
		if rows[i].Mode == "" {
			rows[i].Mode = config.ModeLive
		}
		if rows[i].CdmType == "" {
			rows[i].CdmType = config.CdmWidevine
		}
		if rows[i].CdmMode == "" {
			rows[i].CdmMode = config.CdmInternal
		}
	}
	return rows, nil
}

func syncSummary(kind string, oldList, newList []config.Stream) string {
	oldMap := map[string]config.Stream{}
	for _, st := range oldList {
		oldMap[st.Id] = st
	}
	newMap := map[string]config.Stream{}
	for _, st := range newList {
		newMap[st.Id] = st
	}

	var b strings.Builder
	added, updated, removed := 0, 0, 0
	for _, st := range newList {
		prev, ok := oldMap[st.Id]
		if !ok {
			added++
			fmt.Fprintf(&b, "+ %s (%s)\n", st.Name, st.Id)
			continue
		}
		if streamSyncChanged(prev, st) {
			updated++
			fmt.Fprintf(&b, "~ %s (%s)\n", st.Name, st.Id)
		}
	}
	for _, st := range oldList {
		if _, ok := newMap[st.Id]; !ok {
			removed++
			fmt.Fprintf(&b, "- %s (%s)\n", st.Name, st.Id)
		}
	}
	if added == 0 && updated == 0 && removed == 0 {
		if kind == "events" {
			return "No change in events"
		}
		return "No change in channels"
	}
	head := fmt.Sprintf("Added: %d  Updated: %d  Removed: %d\n", added, updated, removed)
	return head + b.String()
}

func streamSyncChanged(a, b config.Stream) bool {
	return a.Name != b.Name ||
		a.Manifest != b.Manifest ||
		a.ManifestScript != b.ManifestScript ||
		a.Cdm != b.Cdm ||
		a.UseCdm != b.UseCdm ||
		a.SessionManifest != b.SessionManifest ||
		a.Start != b.Start ||
		a.End != b.End ||
		a.Video != b.Video ||
		a.OnDemand != b.OnDemand ||
		a.Autostart != b.Autostart ||
		!equalStrings(a.Keys, b.Keys)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func filterStreams(list []config.Stream, events bool) []config.Stream {
	out := make([]config.Stream, 0, len(list))
	for _, st := range list {
		if st.IsEvent == events {
			out = append(out, st)
		}
	}
	return out
}

func preserveStreamMedia(prev, next config.Stream) config.Stream {
	if strings.TrimSpace(next.Manifest) == "" && strings.TrimSpace(prev.Manifest) != "" {
		next.Manifest = prev.Manifest
	}
	if strings.TrimSpace(next.ManifestScript) == "" && strings.TrimSpace(prev.ManifestScript) != "" {
		next.ManifestScript = prev.ManifestScript
	}
	if strings.TrimSpace(next.Cdm) == "" && strings.TrimSpace(prev.Cdm) != "" {
		next.Cdm = prev.Cdm
	}
	if len(next.Keys) == 0 && len(prev.Keys) > 0 {
		next.Keys = append([]string{}, prev.Keys...)
	}
	if !next.UseCdm && prev.UseCdm && (strings.TrimSpace(next.Manifest) != "" || len(next.Keys) > 0 || strings.TrimSpace(next.Cdm) != "") {
		next.UseCdm = true
	}
	if !next.SessionManifest && prev.SessionManifest && (strings.TrimSpace(next.Manifest) != "" || strings.TrimSpace(next.ManifestScript) != "") {
		next.SessionManifest = true
	}
	if prev.Autostart && !next.Autostart {
		if strings.TrimSpace(next.Manifest) != "" && (len(next.Keys) > 0 || next.UseCdm) {
			next.Autostart = true
		}
	}
	if strings.TrimSpace(next.Video) == "" && strings.TrimSpace(prev.Video) != "" {
		next.Video = prev.Video
	}
	if strings.TrimSpace(next.Audio) == "" && strings.TrimSpace(prev.Audio) != "" {
		next.Audio = prev.Audio
	}
	return next
}

func mergeScriptStreams(existing []config.Stream, incoming []config.Stream, events, autoremove bool) []config.Stream {
	kept := make([]config.Stream, 0, len(existing))
	for _, st := range existing {
		if st.IsEvent != events {
			kept = append(kept, st)
		}
	}

	byID := map[string]config.Stream{}
	order := make([]string, 0, len(existing))
	for _, st := range existing {
		if st.IsEvent != events {
			continue
		}
		byID[st.Id] = st
		order = append(order, st.Id)
	}

	seen := map[string]bool{}
	merged := make([]config.Stream, 0, len(incoming)+len(byID))
	for _, id := range order {
		st := byID[id]
		if st.IgnoreUpdate {
			merged = append(merged, st)
			seen[id] = true
			continue
		}
		found := false
		for _, n := range incoming {
			if n.Id == id {
				n.IsEvent = events
				n.IgnoreUpdate = st.IgnoreUpdate
				n = preserveStreamMedia(st, n)
				merged = append(merged, n)
				seen[id] = true
				found = true
				break
			}
		}
		if !found {
			if !autoremove || keepManagedStream(st) {
				merged = append(merged, st)
				seen[id] = true
			}
		}
	}
	for _, n := range incoming {
		if seen[n.Id] {
			continue
		}
		n.IsEvent = events
		merged = append(merged, n)
	}
	return append(kept, merged...)
}

func keepManagedStream(st config.Stream) bool {
	if st.IgnoreUpdate {
		return true
	}
	if strings.TrimSpace(st.Manifest) != "" || len(st.Keys) > 0 {
		return true
	}
	return false
}

func (s *Server) acChannelsUpdateRequest(w http.ResponseWriter, req actionRequest) {
	p := s.providerOrError(w, req.ProviderId)
	if p == nil {
		return
	}
	raw, err := s.runProviderScript(p, "channels")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rows, err := decodeScriptStreams(raw, "Channels")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for i := range rows {
		rows[i].IsEvent = false
	}
	preview := mergeScriptStreams(p.Streams, rows, false, p.ChannelsAutoremove)
	summary := syncSummary("channels", filterStreams(p.Streams, false), filterStreams(preview, false))
	s.setPending(p.Id, &pendingUpdate{kind: "channels", streams: rows, summary: summary})
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(summary))
}

func (s *Server) acChannelsUpdateApply(w http.ResponseWriter, req actionRequest) {
	p := s.providerOrError(w, req.ProviderId)
	if p == nil {
		return
	}
	u := s.takePending(p.Id, "channels")
	if u == nil {
		http.Error(w, "no pending channels update", http.StatusInternalServerError)
		return
	}
	p.Streams = mergeScriptStreams(p.Streams, u.streams, false, p.ChannelsAutoremove)
	if err := s.store.Put(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mgr.BumpGlobal()
	s.renderJSON(w, "ok")
}

func (s *Server) acEventsRefreshRequest(w http.ResponseWriter, req actionRequest) {
	p := s.providerOrError(w, req.ProviderId)
	if p == nil {
		return
	}
	raw, err := s.runProviderScript(p, "events")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rows, err := decodeScriptStreams(raw, "Events")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for i := range rows {
		rows[i].IsEvent = true
	}
	preview := mergeScriptStreams(p.Streams, rows, true, p.EventsAutoremove)
	summary := syncSummary("events", filterStreams(p.Streams, true), filterStreams(preview, true))
	s.setPending(p.Id, &pendingUpdate{kind: "events", streams: rows, summary: summary})
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(summary))
}

func (s *Server) acEventsRefreshApply(w http.ResponseWriter, req actionRequest) {
	p := s.providerOrError(w, req.ProviderId)
	if p == nil {
		return
	}
	u := s.takePending(p.Id, "events")
	if u == nil {
		http.Error(w, "no pending events update", http.StatusInternalServerError)
		return
	}
	p.Streams = mergeScriptStreams(p.Streams, u.streams, true, p.EventsAutoremove)
	if err := s.store.Put(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mgr.BumpGlobal()
	s.renderJSON(w, "ok")
}

func (s *Server) resolveScriptManifest(p *config.Provider, st *config.Stream) (string, error) {
	extra := splitScriptParams(st.ManifestScript)
	raw, err := s.runProviderScript(p, "manifest", extra...)
	if err != nil {
		return "", err
	}
	var payload struct {
		ManifestUrl string `json:"ManifestUrl"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", err
	}
	return strings.TrimSpace(payload.ManifestUrl), nil
}

func (s *Server) resolveScriptKeys(p *config.Provider, st *config.Stream) ([]string, error) {
	extra := splitScriptParams(st.Cdm)
	raw, err := s.runProviderScript(p, "cdm", extra...)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, ":") {
			continue
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("cdm script returned no keys")
	}
	return out, nil
}

func splitScriptParams(s string) []string {
	fields := strings.Fields(strings.TrimSpace(s))
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if strings.Contains(f, "=") {
			out = append(out, f)
		}
	}
	return out
}
