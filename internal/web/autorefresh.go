package web

import (
	"log"
	"strings"
	"time"

	"drmsonic/internal/config"
)

func (s *Server) eventsAutorefreshLoop() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.sweepEventsAutorefresh()
		}
	}
}

func (s *Server) sweepEventsAutorefresh() {
	now := time.Now()
	for _, p := range s.store.Providers() {
		if p == nil || !p.EventsAutorefresh || strings.TrimSpace(p.Script) == "" {
			continue
		}
		period := p.EventsRefreshPeriod
		if period <= 0 {
			period = 60
		}
		s.refreshMu.Lock()
		last := s.lastEventsRefresh[p.Id]
		if !last.IsZero() && now.Sub(last) < time.Duration(period)*time.Second {
			s.refreshMu.Unlock()
			continue
		}
		if s.refreshBusy[p.Id] {
			s.refreshMu.Unlock()
			continue
		}
		s.refreshBusy[p.Id] = true
		s.refreshMu.Unlock()

		changed, err := s.autoApplyEvents(p)
		s.refreshMu.Lock()
		s.lastEventsRefresh[p.Id] = time.Now()
		s.refreshBusy[p.Id] = false
		s.refreshMu.Unlock()
		if err != nil {
			log.Printf("%s: events autorefresh: %v", p.Id, err)
			continue
		}
		if changed {
			log.Printf("%s: events autorefresh applied", p.Id)
		}
	}
}

func (s *Server) autoApplyEvents(p *config.Provider) (bool, error) {
	raw, err := s.runProviderScript(p, "events")
	if err != nil {
		return false, err
	}
	rows, err := decodeScriptStreams(raw, "Events")
	if err != nil {
		return false, err
	}
	for i := range rows {
		rows[i].IsEvent = true
	}
	fresh := s.store.Provider(p.Id)
	if fresh == nil {
		return false, nil
	}
	before := filterStreams(fresh.Streams, true)
	merged := mergeScriptStreams(fresh.Streams, rows, true, fresh.EventsAutoremove)
	if syncSummary("events", before, filterStreams(merged, true)) == "No change in events" {
		return false, nil
	}
	fresh.Streams = merged
	if err := s.store.Put(fresh); err != nil {
		return false, err
	}
	s.mgr.BumpGlobal()
	s.startAutostartStreams(fresh)
	return true, nil
}

func (s *Server) startAutostartStreams(p *config.Provider) {
	now := time.Now().Unix()
	for i := range p.Streams {
		st := &p.Streams[i]
		if !st.Autostart {
			continue
		}
		ch := s.mgr.Get(p.Id, st.Id)
		if ch != nil && ch.Running {
			continue
		}
		if st.IsEvent && st.Start > 0 {
			if now < st.Start || (st.End > 0 && now > st.End) {
				continue
			}
		}
		s.startChannel(p, st)
		log.Printf("autostart %s/%s", p.Id, st.Id)
	}
}
