package web

import (
	"testing"
	"time"

	"drmsonic/internal/config"
)

func TestOnDemandSweep(t *testing.T) {
	prov := &config.Provider{
		Id:               "channels",
		Name:             "channels",
		RunningMode:      config.RunInternalRemuxer,
		OutputMode:       config.OutDirectHLS,
		PlaylistDuration: 90,
	}
	prov.Streams = []config.Stream{
		{Id: "ondemand", Name: "ondemand", Mode: config.ModeLive, OnDemand: true},
		{Id: "autostart", Name: "autostart", Mode: config.ModeLive, OnDemand: true, Autostart: true},
		{Id: "always", Name: "always", Mode: config.ModeLive},
	}
	s := testServer(t, Options{}, nil, prov)

	for _, id := range []string{"ondemand", "autostart", "always"} {
		s.mgr.Start("channels", id)
	}

	s.media.touch("channels", "ondemand", "")
	s.media.access["channels/ondemand"] = time.Now().Add(-5 * time.Minute)
	s.media.access["channels/autostart"] = time.Now().Add(-5 * time.Minute)

	s.sweepOnDemand()

	if ch := s.mgr.Get("channels", "ondemand"); ch == nil || ch.Running {
		t.Fatalf("idle on-demand stream kept running")
	}
	if ch := s.mgr.Get("channels", "autostart"); ch == nil || !ch.Running {
		t.Fatalf("autostart stream was stopped")
	}
	if ch := s.mgr.Get("channels", "always"); ch == nil || !ch.Running {
		t.Fatalf("non-on-demand stream was stopped")
	}
}

func TestOnDemandSweepKeepsRecentViewer(t *testing.T) {
	prov := &config.Provider{
		Id:               "channels",
		Name:             "channels",
		RunningMode:      config.RunInternalRemuxer,
		OutputMode:       config.OutDirectHLS,
		PlaylistDuration: 90,
	}
	prov.Streams = []config.Stream{{Id: "ondemand", Name: "ondemand", Mode: config.ModeLive, OnDemand: true}}
	s := testServer(t, Options{}, nil, prov)
	s.mgr.Start("channels", "ondemand")

	s.media.touch("channels", "ondemand", "")

	s.sweepOnDemand()

	if ch := s.mgr.Get("channels", "ondemand"); ch == nil || !ch.Running {
		t.Fatalf("recently viewed stream was stopped")
	}
}

func TestOnDemandIdleFloor(t *testing.T) {
	prov := &config.Provider{PlaylistDuration: 15}
	if got := onDemandIdle(prov); got < time.Minute {
		t.Fatalf("idle limit %s, want at least a minute", got)
	}
	prov.PlaylistDuration = 600
	if got := onDemandIdle(prov); got != 600*time.Second {
		t.Fatalf("idle limit %s, want 600s", got)
	}
}

func TestAutostartSkipsEndedBroadcast(t *testing.T) {
	prov := &config.Provider{Id: "willow", Name: "willow", RunningMode: config.RunInternalRemuxer, OutputMode: config.OutDirectHLS}
	prov.Streams = []config.Stream{{Id: "ended", Name: "ended", Mode: config.ModeLive, Autostart: true}}
	s := testServer(t, Options{}, nil, prov)
	s.mgr.Ensure("willow", "ended").StatusLine = "Broadcast ended"

	s.startAutostartStreams(prov)
	if ch := s.mgr.Get("willow", "ended"); ch == nil || ch.Running {
		t.Fatalf("ended broadcast was autostarted again")
	}

	prov.RestartFinished = true
	s.startAutostartStreams(prov)
	if ch := s.mgr.Get("willow", "ended"); ch == nil || !ch.Running {
		t.Fatalf("RestartFinished did not restart the ended broadcast")
	}
	s.StopAll()
}
