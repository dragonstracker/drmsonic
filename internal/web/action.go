package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"drmsonic/internal/config"
	"drmsonic/internal/state"
)

type actionRequest struct {
	Action       string           `json:"Action"`
	ProviderId   string           `json:"ProviderId"`
	ProviderName string           `json:"ProviderName"`
	Streams      []map[string]any `json:"Streams"`
	Provider     map[string]any   `json:"Provider"`
	User         map[string]any   `json:"User"`
	Users        []map[string]any `json:"Users"`
	Raw          map[string]any   `json:"-"`
}

const maxActionBody = 16 << 20

func (s *Server) actionAllowed(r *http.Request, action string) bool {
	u, ok := s.currentUser(r)
	if !ok {
		return !s.authEnabled()
	}
	if u.IsAdmin {
		return true
	}
	if !u.HasWebAccess {
		return false
	}
	switch action {
	case "getstatus", "getconf", "getmonitoring", "getrecordings", "getreplays", "getevents",
		"start", "stop", "restart", "refresh", "refreshkeys", "flushkeys":
		return true
	default:
		return false
	}
}

func (r *actionRequest) userPayload() map[string]any {
	if r.User != nil {
		return r.User
	}
	if len(r.Users) > 0 {
		return r.Users[0]
	}
	return nil
}

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "invalid action", http.StatusInternalServerError)
		return
	}
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
		return
	}
	if !s.csrfOK(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var raw map[string]any
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxActionBody))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		http.Error(w, "bad request body", http.StatusInternalServerError)
		return
	}

	req := actionRequest{Raw: raw}
	req.Action = str(mapGet(raw, "Action"))
	req.ProviderId = str(mapGet(raw, "ProviderId"))
	req.ProviderName = str(mapGet(raw, "ProviderName"))
	if list, ok := raw["Streams"].([]any); ok {
		for _, e := range list {
			if m, ok := e.(map[string]any); ok {
				req.Streams = append(req.Streams, m)
			}
		}
	}
	if list, ok := raw["Users"].([]any); ok {
		for _, e := range list {
			if m, ok := e.(map[string]any); ok {
				req.Users = append(req.Users, m)
			}
		}
	}
	if m, ok := raw["Provider"].(map[string]any); ok {
		req.Provider = m
	}
	if m, ok := raw["User"].(map[string]any); ok {
		req.User = m
	}

	if req.Action == "" {
		http.Error(w, "invalid action", http.StatusInternalServerError)
		return
	}
	if !s.actionAllowed(r, req.Action) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	switch req.Action {
	case "getstatus":
		s.acGetStatus(w, r, req)
	case "getconf":
		s.acGetConf(w, req)
	case "getmonitoring":
		s.acGetMonitoring(w, req)
	case "getrecordings":
		s.renderJSON(w, []config.Recording{})
	case "getreplays":
		s.renderJSON(w, []config.Recording{})
	case "getevents":
		s.renderJSON(w, map[string]any{"Entries": []any{}, "Timezone": "UTC"})
	case "start", "stop", "restart":
		s.acStartStop(w, req)
	case "save":
		s.acSave(w, req)
	case "updatestream":
		s.acUpdateStream(w, req)
	case "addchannel":
		s.acAddChannel(w, req)
	case "deletestream":
		s.acDeleteStream(w, req)
	case "renamechannel":
		s.acRenameChannel(w, req)
	case "addprovider":
		s.acAddProvider(w, req)
	case "deleteprovider":
		s.acDeleteProvider(w, req)
	case "renameprovider":
		s.acRenameProvider(w, req)
	case "refresh", "rescanproviders":
		if err := s.store.Load(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.mgr.BumpGlobal()
		s.renderJSON(w, "ok")
	case "updateglobal", "applyglobalnetwork":
		s.acUpdateGlobal(w, req)
	case "massupdate", "massupdateselection", "massupdateparams", "massupdateset", "massupdateunset":
		s.acMassUpdate(w, req)
	case "adduser":
		s.acAddUser(w, req)
	case "updateuser":
		s.acUpdateUser(w, req)
	case "saveusers":
		s.acSaveUsers(w, req)
	case "deleteuser":
		s.acDeleteUser(w, req)
	case "renameuser":
		s.acRenameUser(w, req)
	case "addrec", "deleterec", "deletereplay", "stoprec":
		s.renderJSON(w, "ok")
	case "refreshkeys", "flushkeys":
		s.mgr.BumpGlobal()
		s.renderJSON(w, "ok")
	case "channelsupdaterequest":
		s.acChannelsUpdateRequest(w, req)
	case "channelsupdateapply":
		s.acChannelsUpdateApply(w, req)
	case "eventsrefreshrequest":
		s.acEventsRefreshRequest(w, req)
	case "eventsrefreshapply":
		s.acEventsRefreshApply(w, req)
	case "export":
		s.acExport(w, req)
	case "import":
		s.acImport(w, req)
	case "shutdown":
		s.renderJSON(w, "ok")
		go s.requestShutdown()
	default:
		http.Error(w, "invalid action", http.StatusInternalServerError)
	}
}

func (s *Server) acGetStatus(w http.ResponseWriter, r *http.Request, req actionRequest) {
	status := state.BuildStatus(s.store, s.mgr, s.baseFor(r))
	if req.ProviderId != "" && req.ProviderId != "all" {
		filtered := status.Providers[:0]
		for _, p := range status.Providers {
			if p.Id == req.ProviderId {
				filtered = append(filtered, p)
			}
		}
		status.Providers = filtered
	}
	ids := requestedIDs(req)
	if len(ids) > 0 && !(len(ids) == 1 && ids[0] == "all") {
		want := make(map[string]bool, len(ids))
		for _, id := range ids {
			want[id] = true
		}
		for i := range status.Providers {
			kept := status.Providers[i].Streams[:0]
			for _, st := range status.Providers[i].Streams {
				if want[st.Id] {
					kept = append(kept, st)
				}
			}
			status.Providers[i].Streams = kept
		}
	}
	s.renderJSON(w, status)
}

func (s *Server) acGetConf(w http.ResponseWriter, req actionRequest) {
	p := s.providerOrError(w, req.ProviderId)
	if p == nil {
		return
	}
	ids := requestedIDs(req)
	out := providerToMap(p, false)
	streams := make([]map[string]any, 0, len(p.Streams))
	if len(ids) > 0 {
		for i := range p.Streams {
			st := &p.Streams[i]
			if !matchIDs(ids, st.Id) {
				continue
			}
			streams = append(streams, map[string]any{
				"Id":     st.Id,
				"Stream": streamToMap(st),
			})
		}
	}
	out["Streams"] = streams
	s.renderJSON(w, out)
}

func (s *Server) acGetMonitoring(w http.ResponseWriter, req actionRequest) {
	providers := s.store.Providers()
	readers := make([]any, 0)
	var totalIn float64
	for _, p := range providers {
		for i := range p.Streams {
			st := &p.Streams[i]
			if ch := s.mgr.Get(p.Id, st.Id); ch != nil && ch.Running {
				totalIn += ch.BwIn
				readers = append(readers, map[string]any{
					"User":         "",
					"Ip":           "",
					"UserAgent":    "",
					"StreamName":   fmt.Sprintf("%s/%s", p.Id, st.Id),
					"ProviderName": p.Name,
					"Bw":           ch.BwIn,
					"BwColor":      "white",
					"Errors":       "",
					"ErrorsColor":  "white",
					"Uptime":       ch.StatusText(),
					"Type":         "hls",
				})
			}
		}
	}
	s.renderJSON(w, map[string]any{
		"Readers":      readers,
		"Memory":       runtimeMemory(),
		"CpuLoad":      0.0,
		"CpuLoadColor": "green",
		"TotalBwIn":    totalIn,
		"TotalBwOut":   totalIn,
		"Timezone":     "UTC",
	})
}

func (s *Server) acSave(w http.ResponseWriter, req actionRequest) {
	if req.ProviderId == "" {
		for _, p := range s.store.Providers() {
			if err := s.store.Put(p); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if err := s.store.SaveUsers(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.mgr.BumpGlobal()
		s.renderJSON(w, "ok")
		return
	}
	p := s.providerOrError(w, req.ProviderId)
	if p == nil {
		return
	}
	if err := s.store.Put(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mgr.BumpGlobal()
	s.renderJSON(w, "ok")
}

func (s *Server) acUpdateStream(w http.ResponseWriter, req actionRequest) {
	p := s.providerOrError(w, req.ProviderId)
	if p == nil {
		return
	}
	for _, payload := range req.Streams {
		id := str(mapGet(payload, "Id"))
		if id == "" || id == "all" {
			continue
		}
		st := p.FindStream(id)
		if st == nil {
			continue
		}
		applyStreamPayload(st, payload)
		if ch := s.mgr.Get(p.Id, st.Id); ch != nil && ch.Running {
			s.restartChannel(p, st)
		}
	}
	if err := s.store.Put(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mgr.BumpGlobal()
	s.renderJSON(w, "ok")
}

func (s *Server) acAddChannel(w http.ResponseWriter, req actionRequest) {
	p := s.providerOrError(w, req.ProviderId)
	if p == nil {
		return
	}
	name := str(mapGet(req.Raw, "Name"))
	if name == "" && len(req.Streams) > 0 {
		name = str(mapGet(req.Streams[0], "Name"))
	}
	if name == "" {
		http.Error(w, "missing stream name", http.StatusInternalServerError)
		return
	}
	id := config.Slug(name)
	if p.FindStream(id) != nil {
		http.Error(w, "stream already exists", http.StatusInternalServerError)
		return
	}
	p.Streams = append(p.Streams, config.Stream{
		Id:           id,
		Name:         name,
		Mode:         config.ModeLive,
		RunningMode:  p.EffectiveRunningMode(),
		OutputMode:   p.EffectiveOutputMode(),
		NetworkScope: "script,manifest,media",
		CdmType:      config.CdmWidevine,
		CdmMode:      config.CdmInternal,
		NameColor:    "white",
		StatusColor:  "red",
	})
	if err := s.store.Put(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mgr.BumpGlobal()
	s.renderJSON(w, "ok")
}

func (s *Server) acStartStop(w http.ResponseWriter, req actionRequest) {
	p := s.providerOrError(w, req.ProviderId)
	if p == nil {
		return
	}
	ids := requestedIDs(req)
	autostartOnly := false
	filtered := ids[:0]
	for _, id := range ids {
		if id == "autostart" {
			autostartOnly = true
			continue
		}
		filtered = append(filtered, id)
	}
	ids = filtered

	payloadByID := map[string]map[string]any{}
	for _, payload := range req.Streams {
		id := str(mapGet(payload, "Id"))
		if id == "" || id == "all" || id == "autostart" || strings.Contains(id, ",") {
			continue
		}
		payloadByID[id] = payload
	}

	dirty := false
	for i := range p.Streams {
		st := &p.Streams[i]
		if autostartOnly {
			if !st.Autostart {
				continue
			}
		} else if !matchIDs(ids, st.Id) {
			continue
		}
		if payload, ok := payloadByID[st.Id]; ok && (req.Action == "start" || req.Action == "restart") {
			applyStreamPayload(st, payload)
			dirty = true
		}
		switch req.Action {
		case "start":
			s.startChannel(p, st)
		case "restart":
			s.stopChannel(p.Id, st.Id)
			s.startChannel(p, st)
		default:
			s.stopChannel(p.Id, st.Id)
			s.media.drop(p.Id, st.Id)
		}
	}
	if dirty {
		if err := s.store.Put(p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	s.mgr.BumpGlobal()
	s.renderJSON(w, "ok")
}

func (s *Server) acDeleteStream(w http.ResponseWriter, req actionRequest) {
	p := s.providerOrError(w, req.ProviderId)
	if p == nil {
		return
	}
	ids := requestedIDs(req)
	kept := p.Streams[:0]
	for _, st := range p.Streams {
		if matchIDs(ids, st.Id) {
			s.stopChannel(p.Id, st.Id)
			s.media.drop(p.Id, st.Id)
			continue
		}
		kept = append(kept, st)
	}
	p.Streams = kept
	if err := s.store.Put(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mgr.BumpGlobal()
	s.renderJSON(w, "ok")
}

func (s *Server) acRenameChannel(w http.ResponseWriter, req actionRequest) {
	p := s.providerOrError(w, req.ProviderId)
	if p == nil {
		return
	}
	ids := requestedIDs(req)
	name := str(mapGet(req.Raw, "Name"))
	if name == "" && len(req.Streams) > 0 {
		name = str(mapGet(req.Streams[0], "Name"))
	}
	for i := range p.Streams {
		if matchIDs(ids, p.Streams[i].Id) {
			p.Streams[i].Name = name
			p.Streams[i].Id = config.Slug(name)
		}
	}
	if err := s.store.Put(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mgr.BumpGlobal()
	s.renderJSON(w, "ok")
}

func (s *Server) acAddProvider(w http.ResponseWriter, req actionRequest) {
	name := str(mapGet(req.Raw, "Name"))
	if name == "" {
		name = req.ProviderName
	}
	if name == "" {
		name = str(mapGet(req.Provider, "Name"))
	}
	if name == "" {
		http.Error(w, "missing provider name", http.StatusInternalServerError)
		return
	}
	p := &config.Provider{
		Id:                   config.Slug(name),
		Name:                 name,
		RunningMode:          config.RunInternalRemuxer,
		OutputMode:           config.OutDirectHLS,
		NetworkScope:         "script,manifest,media",
		EventsRefreshPeriod:  60,
		HttpGetRetries:       2,
		StallDetectTimeout:   20,
		MaxConcurrentStreams: 10,
		NbAnnouncedFragments: 6,
		PlaylistDuration:     30,
		HlsFragmentsDuration: 2,
		Streams:              []config.Stream{},
	}
	if err := s.store.Put(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mgr.BumpGlobal()
	s.renderJSON(w, p.Id)
}

func (s *Server) acDeleteProvider(w http.ResponseWriter, req actionRequest) {
	id := req.ProviderId
	if id == "" {
		id = str(mapGet(req.Raw, "Id"))
	}
	if id == "" {
		http.Error(w, "missing provider", http.StatusInternalServerError)
		return
	}
	if p := s.store.Provider(id); p != nil {
		for i := range p.Streams {
			s.stopChannel(p.Id, p.Streams[i].Id)
			s.media.drop(p.Id, p.Streams[i].Id)
		}
	}
	s.mgr.StopProvider(id)
	if err := s.store.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mgr.BumpGlobal()
	s.renderJSON(w, "ok")
}

func (s *Server) acRenameProvider(w http.ResponseWriter, req actionRequest) {
	p := s.providerOrError(w, req.ProviderId)
	if p == nil {
		return
	}
	name := str(mapGet(req.Raw, "Name"))
	if name == "" {
		name = req.ProviderName
	}
	if name == "" {
		http.Error(w, "missing provider name", http.StatusInternalServerError)
		return
	}
	old := p.Id
	p.Name = name
	p.Id = config.Slug(name)
	if old != p.Id {
		if err := s.store.Delete(old); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := s.store.Put(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mgr.BumpGlobal()
	s.renderJSON(w, p.Id)
}

func (s *Server) acUpdateGlobal(w http.ResponseWriter, req actionRequest) {
	p := s.providerOrError(w, req.ProviderId)
	if p == nil {
		return
	}
	stringsFields := []struct {
		key string
		dst *string
	}{
		{"Script", &p.Script},
		{"Proxy", &p.Proxy},
		{"Bind", &p.Bind},
		{"Doh", &p.Doh},
		{"NetworkScope", &p.NetworkScope},
		{"UserAgent", &p.UserAgent},
		{"RunningMode", &p.RunningMode},
		{"OutputMode", &p.OutputMode},
		{"EpgTimezone", &p.EpgTimezone},
		{"VmxUniqueId", &p.VmxUniqueId},
		{"PipeOutputCmdFormated", &p.PipeOutputCmdFormated},
	}
	for _, f := range stringsFields {
		if v, ok := req.Raw[f.key]; ok {
			*f.dst = str(v)
		}
	}

	boolFields := []struct {
		key string
		dst *bool
	}{
		{"XForwardedFor", &p.XForwardedFor},
		{"EventsAutorefresh", &p.EventsAutorefresh},
		{"EventsAutoremove", &p.EventsAutoremove},
		{"ChannelsAutoremove", &p.ChannelsAutoremove},
		{"AlwaysResetSession", &p.AlwaysResetSession},
		{"NoRestartOnError", &p.NoRestartOnError},
		{"RestartFinished", &p.RestartFinished},
		{"NoRestartOnTrackChange", &p.NoRestartOnTrackChange},
		{"ReuseEventIndex", &p.ReuseEventIndex},
		{"NoWaitFullPlaylist", &p.NoWaitFullPlaylist},
		{"ContinuousPlayback", &p.ContinuousPlayback},
		{"IgnoreStaticDash", &p.IgnoreStaticDash},
		{"UseDashDelay", &p.UseDashDelay},
	}
	for _, f := range boolFields {
		if v, ok := req.Raw[f.key]; ok {
			*f.dst = boolOf(v)
		}
	}

	intFields := []struct {
		key string
		dst *int
	}{
		{"MaxConcurrentStreams", &p.MaxConcurrentStreams},
		{"RestartDelay", &p.RestartDelay},
		{"NbAnnouncedFragments", &p.NbAnnouncedFragments},
		{"HlsFragmentsDuration", &p.HlsFragmentsDuration},
		{"AutorestartPeriod", &p.AutorestartPeriod},
		{"PlaybackDelay", &p.PlaybackDelay},
		{"RandomAutostartPeriod", &p.RandomAutostartPeriod},
		{"SequencialAutostartPeriod", &p.SequencialAutostartPeriod},
		{"SequentialAutostartPeriod", &p.SequencialAutostartPeriod},
		{"EventsRefreshPeriod", &p.EventsRefreshPeriod},
		{"HttpGetRetries", &p.HttpGetRetries},
		{"StallDetectTimeout", &p.StallDetectTimeout},
	}
	for _, f := range intFields {
		if v, ok := req.Raw[f.key]; ok {
			*f.dst = intOf(v)
		}
	}

	if v, ok := req.Raw["PlaylistDuration"]; ok {
		p.PlaylistDuration = floatOf(v)
	}
	if v, ok := req.Raw["Headers"]; ok {
		p.Headers = stringSlice(v)
	}

	if err := s.store.Put(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.restartRunning(p)
	s.mgr.BumpGlobal()
	s.renderJSON(w, "ok")
}

func (s *Server) acMassUpdate(w http.ResponseWriter, req actionRequest) {
	p := s.providerOrError(w, req.ProviderId)
	if p == nil {
		return
	}
	ids := requestedIDs(req)
	field := str(mapGet(req.Raw, "MassUpdateParam"))
	if field == "" {
		field = str(mapGet(req.Raw, "Field"))
	}
	value := mapGet(req.Raw, "Value")
	if v := mapGet(req.Raw, "MassUpdateValue"); v != nil {
		value = v
	}
	set := !strings.EqualFold(str(value), "unset") || req.Action == "massupdateset"
	if req.Action == "massupdateunset" {
		set = false
	}

	for i := range p.Streams {
		if !matchIDs(ids, p.Streams[i].Id) {
			continue
		}
		if field == "" {
			continue
		}
		applyDynamicField(&p.Streams[i], field, set, value)
	}
	if err := s.store.Put(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mgr.BumpGlobal()
	s.renderJSON(w, "ok")
}

func (s *Server) acAddUser(w http.ResponseWriter, req actionRequest) {
	src := req.userPayload()
	name := str(mapGet(src, "Username"))
	if name == "" {
		name = str(mapGet(req.Raw, "Username"))
	}
	if name == "" {
		http.Error(w, "missing username", http.StatusInternalServerError)
		return
	}
	users := s.store.Users()
	for _, u := range users {
		if u.Username == name {
			http.Error(w, "user already exists", http.StatusInternalServerError)
			return
		}
	}
	pass := strings.TrimSpace(str(mapGet(src, "Password")))
	if pass == "" {
		http.Error(w, "password required", http.StatusInternalServerError)
		return
	}
	tok := config.NewStreamToken()
	isAdmin := false
	if v, ok := src["IsAdmin"]; ok {
		isAdmin = boolOf(v)
	}
	hasWeb := true
	if v, ok := src["HasWebAccess"]; ok {
		hasWeb = boolOf(v)
	}
	noPlaylist := false
	if v, ok := src["NoPlaylist"]; ok {
		noPlaylist = boolOf(v)
	}
	users = append(users, config.User{
		Username:     name,
		Password:     pass,
		StreamToken:  tok,
		IsAdmin:      isAdmin,
		HasWebAccess: hasWeb,
		NoPlaylist:   noPlaylist,
		ProviderIds:  stringSlice(mapGet(src, "ProviderIds")),
	})
	s.store.SetUsers(users)
	if err := s.store.SaveUsers(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderJSON(w, "ok")
}

func applyUserFields(u *config.User, src map[string]any) {
	if v, ok := src["Password"]; ok {
		if pass := str(v); pass != "" && pass != "hidden" {
			u.Password = pass
			if u.StreamToken == "" {
				u.StreamToken = config.NewStreamToken()
			}
		}
	}
	if u.Password != "" && u.StreamToken == "" {
		u.StreamToken = config.NewStreamToken()
	}
	if v, ok := src["IsAdmin"]; ok {
		u.IsAdmin = boolOf(v)
	}
	if v, ok := src["HasWebAccess"]; ok {
		u.HasWebAccess = boolOf(v)
	}
	if v, ok := src["NoPlaylist"]; ok {
		u.NoPlaylist = boolOf(v)
	}
	if v, ok := src["ProviderIds"]; ok {
		u.ProviderIds = stringSlice(v)
	}
	if v, ok := src["Network"]; ok {
		u.Network = str(v)
	}
}

func (s *Server) acUpdateUser(w http.ResponseWriter, req actionRequest) {
	src := req.userPayload()
	name := strings.TrimSpace(str(mapGet(src, "Username")))
	if name == "" {
		http.Error(w, "username required", http.StatusInternalServerError)
		return
	}
	users := s.store.Users()
	found := false
	for i := range users {
		if users[i].Username != name {
			continue
		}
		found = true
		applyUserFields(&users[i], src)
		break
	}
	if !found {
		http.Error(w, "unknown user", http.StatusInternalServerError)
		return
	}
	s.store.SetUsers(users)
	if err := s.store.SaveUsers(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderJSON(w, "ok")
}

func (s *Server) acSaveUsers(w http.ResponseWriter, req actionRequest) {
	users := s.store.Users()
	index := make(map[string]int, len(users))
	for i := range users {
		index[users[i].Username] = i
	}
	for _, src := range req.Users {
		name := strings.TrimSpace(str(mapGet(src, "Username")))
		if name == "" {
			continue
		}
		i, ok := index[name]
		if !ok {
			http.Error(w, "unknown user: "+name, http.StatusInternalServerError)
			return
		}
		applyUserFields(&users[i], src)
	}
	if v, ok := req.Raw["NoAuth"]; ok {
		s.store.SetNoAuth(boolOf(v))
	}
	s.store.SetUsers(users)
	if err := s.store.SaveUsers(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderJSON(w, "ok")
}

func (s *Server) acDeleteUser(w http.ResponseWriter, req actionRequest) {
	src := req.userPayload()
	name := str(mapGet(src, "Username"))
	if name == "" {
		name = str(mapGet(req.Raw, "Username"))
	}
	users := s.store.Users()
	kept := users[:0]
	for _, u := range users {
		if u.Username != name {
			kept = append(kept, u)
		}
	}
	s.store.SetUsers(kept)
	if err := s.store.SaveUsers(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderJSON(w, "ok")
}

func (s *Server) acRenameUser(w http.ResponseWriter, req actionRequest) {
	old := str(mapGet(req.Raw, "Username"))
	if v, ok := req.Raw["OldUsername"]; ok {
		old = str(v)
	}
	if old == "" {
		old = str(mapGet(req.userPayload(), "Username"))
	}
	name := str(mapGet(req.Raw, "Name"))
	if name == "" {
		name = str(mapGet(req.Raw, "NewUsername"))
	}
	if name == "" {
		name = str(mapGet(req.Raw, "NewUserName"))
	}
	if name == "" || old == "" {
		http.Error(w, "missing username", http.StatusInternalServerError)
		return
	}
	if name == old {
		s.renderJSON(w, "ok")
		return
	}
	users := s.store.Users()
	found := false
	for i := range users {
		if users[i].Username == name {
			http.Error(w, "user already exists", http.StatusInternalServerError)
			return
		}
	}
	for i := range users {
		if users[i].Username == old {
			users[i].Username = name
			found = true
		}
	}
	if !found {
		http.Error(w, "unknown user", http.StatusInternalServerError)
		return
	}
	s.store.SetUsers(users)
	if err := s.store.SaveUsers(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderJSON(w, "ok")
}

func (s *Server) acExport(w http.ResponseWriter, req actionRequest) {
	p := s.providerOrError(w, req.ProviderId)
	if p == nil {
		return
	}
	ids := requestedIDs(req)
	out := providerToMap(p, true)
	streams := make([]map[string]any, 0)
	for i := range p.Streams {
		if matchIDs(ids, p.Streams[i].Id) {
			streams = append(streams, streamToMap(&p.Streams[i]))
		}
	}
	out["Streams"] = streams
	s.renderJSON(w, out)
}

func (s *Server) acImport(w http.ResponseWriter, req actionRequest) {
	body, err := importBody(req.Raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p, err := providerFromBody(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := req.ProviderName
	if name == "" {
		name = p.Name
	}
	if name != "" {
		p.Name = name
		p.Id = config.Slug(name)
	}
	if p.Id == "" {
		p.Id = config.Slug(p.Name)
	}
	if err := s.store.Put(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mgr.BumpGlobal()
	s.renderJSON(w, "ok")
}

func (s *Server) providerOrError(w http.ResponseWriter, id string) *config.Provider {
	if id == "" {
		p := s.store.DefaultProvider()
		if p == nil {
			http.Error(w, "invalid provider", http.StatusInternalServerError)
			return nil
		}
		return p
	}
	p := s.store.Provider(id)
	if p == nil {
		http.Error(w, "invalid provider", http.StatusInternalServerError)
		return nil
	}
	return p
}

func requestedIDs(req actionRequest) []string {
	out := []string{}
	for _, m := range req.Streams {
		v := mapGet(m, "Id")
		switch t := v.(type) {
		case string:
			for _, part := range strings.Split(t, ",") {
				part = strings.TrimSpace(part)
				if part != "" {
					out = append(out, part)
				}
			}
		case []any:
			for _, e := range t {
				s := str(e)
				if s != "" {
					out = append(out, s)
				}
			}
		}
	}
	if len(out) == 0 {
		for _, key := range []string{"MassUpdateStreamIds", "StreamIds", "Ids", "Streams"} {
			if v := mapGet(req.Raw, key); v != nil {
				out = append(out, stringSlice(v)...)
				break
			}
		}
	}
	return out
}

func matchIDs(ids []string, id string) bool {
	if len(ids) == 0 {
		return true
	}
	for _, want := range ids {
		if want == "all" || want == id {
			return true
		}
	}
	return false
}

func streamHeadersUI(st *config.Stream) map[string]any {
	manifest := map[string]string{}
	media := map[string]string{}
	for _, line := range st.Headers {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		manifest[strings.TrimSpace(k)] = strings.TrimSpace(v)
		media[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return map[string]any{"Manifest": manifest, "Media": media}
}

func streamToMap(st *config.Stream) map[string]any {
	videoList := st.VideoList
	if videoList == nil {
		videoList = []config.Track{}
	}
	audioList := st.AudioList
	if audioList == nil {
		audioList = []config.Track{}
	}
	subtitlesList := st.SubtitlesList
	if subtitlesList == nil {
		subtitlesList = []config.Track{}
	}
	keys := st.Keys
	if keys == nil {
		keys = []string{}
	}
	return map[string]any{
		"Name":                   st.Name,
		"Id":                     st.Id,
		"Mode":                   st.Mode,
		"IsEvent":                st.IsEvent,
		"Manifest":               st.Manifest,
		"ManifestUrl":            st.Manifest,
		"SessionManifest":        st.SessionManifest,
		"ManifestScript":         st.ManifestScript,
		"Headers":                streamHeadersUI(st),
		"Video":                  st.Video,
		"Audio":                  st.Audio,
		"Subtitles":              st.Subtitles,
		"VideoList":              videoList,
		"AudioList":              audioList,
		"SubtitlesList":          subtitlesList,
		"Keys":                   keys,
		"UseCdm":                 st.UseCdm,
		"CdmType":                st.CdmType,
		"CdmMode":                st.CdmMode,
		"Cdm":                    st.Cdm,
		"PRClientVersion":        st.PRClientVersion,
		"PRCustomData":           st.PRCustomData,
		"PRLAVersion":            st.PRLAVersion,
		"VmxUniqueId":            st.VmxUniqueId,
		"RunningMode":            st.RunningMode,
		"OutputMode":             st.OutputMode,
		"ModeOverride":           st.ModeOverride,
		"MultiTS":                st.MultiTS,
		"Autostart":              st.Autostart,
		"OnDemand":               st.OnDemand,
		"SpeedUp":                st.SpeedUp,
		"NetworkOverride":        st.NetworkOverride,
		"NetworkScope":           st.NetworkScope,
		"Bind":                   st.Bind,
		"Proxy":                  st.Proxy,
		"Doh":                    st.Doh,
		"RecordEvent":            st.RecordEvent,
		"IgnoreUpdate":           st.IgnoreUpdate,
		"FixIvSize":              st.FixIvSize,
		"TimeRange":              st.TimeRange,
		"RangeStartTime":         st.RangeStartTime,
		"RangeEndTime":           st.RangeEndTime,
		"Start":                  st.Start,
		"End":                    st.End,
		"Output":                 st.Output,
		"PipeOutputParams":       st.PipeOutputParams,
		"ProtoOutputParams":      st.ProtoOutputParams,
		"NoRestartOnError":       st.NoRestartOnError,
		"NoRestartOnTrackChange": st.NoRestartOnTrackChange,
		"NoWaitFullPlaylist":     st.NoWaitFullPlaylist,
		"PlaylistDuration":       st.PlaylistDuration,
		"Running":                st.Running,
		"Status":                 st.Status,
		"StatusColor":            st.StatusColor,
	}
}

func providerToMap(p *config.Provider, withStreams bool) map[string]any {
	m := map[string]any{
		"Id":                        p.Id,
		"Name":                      p.Name,
		"RunningMode":               p.EffectiveRunningMode(),
		"OutputMode":                p.EffectiveOutputMode(),
		"Bind":                      p.Bind,
		"Proxy":                     p.Proxy,
		"Doh":                       p.Doh,
		"NetworkScope":              p.NetworkScope,
		"Headers":                   p.Headers,
		"UserAgent":                 p.UserAgent,
		"Script":                    p.Script,
		"VmxUniqueId":               p.VmxUniqueId,
		"XForwardedFor":             p.XForwardedFor,
		"AlwaysResetSession":        p.AlwaysResetSession,
		"PipeOutputCmdFormated":     p.PipeOutputCmdFormated,
		"AutorestartPeriod":         p.AutorestartPeriod,
		"RandomAutostartPeriod":     p.RandomAutostartPeriod,
		"SequencialAutostartPeriod": p.SequencialAutostartPeriod,
		"RestartDelay":              p.RestartDelay,
		"RestartFinished":           p.RestartFinished,
		"PlaylistDuration":          p.PlaylistDuration,
		"HlsFragmentsDuration":      p.HlsFragmentsDuration,
		"NbAnnouncedFragments":      p.NbAnnouncedFragments,
		"PlaybackDelay":             p.PlaybackDelay,
		"ContinuousPlayback":        p.ContinuousPlayback,
		"NoWaitFullPlaylist":        p.NoWaitFullPlaylist,
		"IgnoreStaticDash":          p.IgnoreStaticDash,
		"UseDashDelay":              p.UseDashDelay,
		"HttpGetRetries":            p.HttpGetRetries,
		"StallDetectTimeout":        p.StallDetectTimeout,
		"MaxConcurrentStreams":      p.MaxConcurrentStreams,
		"NoRestartOnError":          p.NoRestartOnError,
		"NoRestartOnTrackChange":    p.NoRestartOnTrackChange,
		"EventsAutorefresh":         p.EventsAutorefresh,
		"EventsAutoremove":          p.EventsAutoremove,
		"EventsRefreshPeriod":       p.EventsRefreshPeriod,
		"ChannelsAutoremove":        p.ChannelsAutoremove,
		"ReuseEventIndex":           p.ReuseEventIndex,
		"EpgTimezone":               p.EpgTimezone,
	}
	if withStreams {
		list := make([]map[string]any, 0, len(p.Streams))
		for i := range p.Streams {
			list = append(list, streamToMap(&p.Streams[i]))
		}
		m["Streams"] = list
	}
	return m
}

func applyStreamPayload(st *config.Stream, m map[string]any) {
	if v, ok := m["Name"]; ok {
		st.Name = str(v)
	}
	if v, ok := m["ManifestUrl"]; ok {
		st.Manifest = str(v)
	}
	if v, ok := m["Manifest"]; ok && st.Manifest == "" {
		st.Manifest = str(v)
	}
	if v, ok := m["Mode"]; ok {
		st.Mode = str(v)
	}
	if v, ok := m["ManifestScript"]; ok {
		st.ManifestScript = str(v)
	}
	if v, ok := m["RunningMode"]; ok {
		st.RunningMode = str(v)
	}
	if v, ok := m["OutputMode"]; ok {
		st.OutputMode = str(v)
	}
	if v, ok := m["VStreamId"]; ok {
		st.Video = str(v)
	}
	if v, ok := m["AStreamId"]; ok {
		st.Audio = str(v)
	}
	if v, ok := m["SStreamId"]; ok {
		st.Subtitles = str(v)
	}
	if v, ok := m["CdmType"]; ok {
		st.CdmType = str(v)
	}
	if v, ok := m["CdmMode"]; ok {
		st.CdmMode = str(v)
	}
	if v, ok := m["Cdm"]; ok {
		st.Cdm = str(v)
	}
	if v, ok := m["PRLAVersion"]; ok {
		st.PRLAVersion = str(v)
	}
	if v, ok := m["PRClientVersion"]; ok {
		st.PRClientVersion = str(v)
	}
	if v, ok := m["PRCustomData"]; ok {
		st.PRCustomData = str(v)
	}
	if v, ok := m["Keys"]; ok {
		st.Keys = splitLines(str(v))
	}
	if v, ok := m["PipeOutputParams"]; ok {
		st.PipeOutputParams = str(v)
	}
	if v, ok := m["ProtoOutputParams"]; ok {
		st.ProtoOutputParams = str(v)
	}
	if v, ok := m["Bind"]; ok {
		st.Bind = str(v)
	}
	if v, ok := m["Proxy"]; ok {
		st.Proxy = str(v)
	}
	if v, ok := m["Doh"]; ok {
		st.Doh = str(v)
	}
	if v, ok := m["NetworkScope"]; ok {
		st.NetworkScope = str(v)
	}
	if v, ok := m["RangeStartTime"]; ok {
		st.RangeStartTime = str(v)
	}
	if v, ok := m["RangeEndTime"]; ok {
		st.RangeEndTime = str(v)
	}
	if v, ok := m["Start"]; ok {
		st.RangeStartTime = str(v)
	}
	if v, ok := m["End"]; ok {
		st.RangeEndTime = str(v)
	}
	if v, ok := m["Output"]; ok {
		st.Output = str(v)
	}

	for _, f := range []struct {
		key string
		dst *bool
	}{
		{"OnDemand", &st.OnDemand},
		{"Autostart", &st.Autostart},
		{"SessionManifest", &st.SessionManifest},
		{"SpeedUp", &st.SpeedUp},
		{"UseCdm", &st.UseCdm},
		{"NetworkOverride", &st.NetworkOverride},
		{"ModeOverride", &st.ModeOverride},
		{"RecordEvent", &st.RecordEvent},
		{"IgnoreUpdate", &st.IgnoreUpdate},
		{"FixIvSize", &st.FixIvSize},
		{"TimeRange", &st.TimeRange},
		{"MultiTS", &st.MultiTS},
		{"NoRestartOnError", &st.NoRestartOnError},
		{"NoRestartOnTrackChange", &st.NoRestartOnTrackChange},
		{"NoWaitFullPlaylist", &st.NoWaitFullPlaylist},
	} {
		if v, ok := m[f.key]; ok {
			*f.dst = boolOf(v)
		}
	}
}

func applyDynamicField(st *config.Stream, field string, set bool, value any) {
	switch strings.ToLower(field) {
	case "autostart":
		st.Autostart = set
	case "ondemand":
		st.OnDemand = set
	case "sessionmanifest":
		st.SessionManifest = set
	case "speedup":
		st.SpeedUp = set
	case "usecdm":
		st.UseCdm = set
	case "recordevent":
		st.RecordEvent = set
	case "networkoverride", "network":
		st.NetworkOverride = set
	case "modeoverride":
		st.ModeOverride = set
	case "ignoreupdate":
		st.IgnoreUpdate = set
	case "fixivsize":
		st.FixIvSize = set
	case "timerange":
		st.TimeRange = set
	case "clearmanifest":
		if set {
			st.Manifest = ""
		}
	case "clearmanifestscript":
		if set {
			st.ManifestScript = ""
		}
	case "clearcdm":
		if set {
			st.Cdm = ""
			st.UseCdm = false
		}
	case "clearoldkeys":
		if set {
			st.Keys = nil
		}
	case "setwidevine":
		st.CdmType = config.CdmWidevine
	case "setplayready":
		st.CdmType = config.CdmPlayReady
	case "setverimatrix":
		st.CdmType = config.CdmVerimatrix
	case "setcdminternal":
		st.CdmMode = config.CdmInternal
	case "setcdmexternal":
		st.CdmMode = config.CdmExternal
	case "selectbestvideo":
		if set {
			st.Video = "max"
		} else {
			st.Video = ""
		}
	case "selectallaudios":
		if set {
			st.Audio = "all"
		} else {
			st.Audio = ""
		}
	case "runningmode":
		if set {
			st.RunningMode = str(value)
		}
	case "outputmode":
		if set {
			st.OutputMode = str(value)
		}
	case "mode":
		if set {
			st.Mode = str(value)
		}
	}
}

func mapGet(m map[string]any, key string) any {
	if m == nil {
		return nil
	}
	if v, ok := m[key]; ok {
		return v
	}
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return nil
}

func str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "true"
		}
		return "false"
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, str(e))
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprint(t)
	}
}

func boolOf(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t == "true" || t == "1" || t == "on"
	case json.Number:
		return t.String() != "0"
	case float64:
		return t != 0
	default:
		return false
	}
}

func intOf(v any) int {
	switch t := v.(type) {
	case nil:
		return 0
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	case bool:
		if t {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func floatOf(v any) float64 {
	switch t := v.(type) {
	case nil:
		return 0
	case json.Number:
		f, _ := t.Float64()
		return f
	case float64:
		return t
	case int:
		return float64(t)
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f
	default:
		return 0
	}
}

func stringSlice(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			s := str(e)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		return splitLines(t)
	default:
		return nil
	}
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}
