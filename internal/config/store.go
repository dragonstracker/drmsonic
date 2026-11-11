package config

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Store struct {
	mu        sync.RWMutex
	dir       string
	providers map[string]*Provider
	users     []User
	noAuth    bool
	confPath  string
	order     []string
}

func NewStore(dir, confPath string) (*Store, error) {
	s := &Store{
		dir:       dir,
		confPath:  confPath,
		providers: make(map[string]*Provider),
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := s.Load(); err != nil {
		return nil, err
	}
	if err := s.EnsureTokensPersisted(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) EnsureTokensPersisted() error {
	s.mu.Lock()
	users, changed := EnsureUserTokens(s.users)
	s.users = users
	s.mu.Unlock()
	if !changed {
		return nil
	}
	return s.SaveUsers()
}

func (s *Store) EnsureDefaultAdmin() (bool, error) {
	s.mu.Lock()
	users, changed := EnsureDefaultAdmin(s.users)
	s.users = users
	s.mu.Unlock()
	if !changed {
		return false, nil
	}
	return true, s.SaveUsers()
}

func (s *Store) Dir() string {
	return s.dir
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}

	providers := make(map[string]*Provider)
	order := make([]string, 0, len(entries))

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		full := filepath.Join(s.dir, e.Name())
		raw, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		var p Provider
		if err := json.Unmarshal(raw, &p); err != nil {
			continue
		}
		if p.Id == "" {
			p.Id = strings.TrimSuffix(e.Name(), ".json")
		}
		if p.Name == "" {
			p.Name = p.Id
		}
		p.Streams = normalizeStreams(p.Streams, &p)
		providers[p.Id] = &p
		order = append(order, p.Id)
	}

	sort.Strings(order)
	s.providers = providers
	s.order = order

	if s.confPath != "" {
		if raw, err := os.ReadFile(s.confPath); err == nil {
			var c Config
			if json.Unmarshal(raw, &c) == nil {
				s.users, _ = EnsureUserTokens(c.Users)
				s.noAuth = c.NoAuth
			}
		}
	}
	return nil
}

func normalizeStreams(in []Stream, p *Provider) []Stream {
	out := make([]Stream, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, st := range in {
		if st.Id == "" {
			st.Id = slug(st.Name)
		}
		if seen[st.Id] {
			continue
		}
		seen[st.Id] = true
		if st.Mode == "" {
			st.Mode = ModeLive
		}
		if st.NetworkScope == "" {
			st.NetworkScope = "script,manifest,media"
		}
		if st.CdmType == "" {
			st.CdmType = CdmWidevine
		}
		if st.CdmMode == "" {
			st.CdmMode = CdmInternal
		}
		if st.StatusColor == "" {
			st.StatusColor = "green"
		}
		if st.NameColor == "" {
			st.NameColor = "white"
		}
		if st.Name == "" {
			st.Name = st.Id
		}
		out = append(out, st)
	}
	return out
}

func (s *Store) Providers() []*Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Provider, 0, len(s.order))
	for _, id := range s.order {
		if p, ok := s.providers[id]; ok {
			out = append(out, p)
		}
	}
	return out
}

func (s *Store) Provider(id string) *Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.providers[id]
}

func (s *Store) DefaultProvider() *Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.order) == 0 {
		return nil
	}
	return s.providers[s.order[0]]
}

func (s *Store) Users() []User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]User, len(s.users))
	copy(out, s.users)
	return out
}

func (s *Store) NoAuth() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.noAuth
}

func (s *Store) SetNoAuth(v bool) {
	s.mu.Lock()
	s.noAuth = v
	s.mu.Unlock()
}

func (s *Store) UserByName(name string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.users {
		if u.Username == name {
			return u, true
		}
	}
	return User{}, false
}

func (s *Store) SetUsers(u []User) {
	s.mu.Lock()
	users, _ := EnsureUserTokens(u)
	s.users = users
	s.mu.Unlock()
}

func (s *Store) UserByToken(token string) (User, bool) {
	if token == "" {
		return User{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.users {
		if u.StreamToken == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(u.StreamToken), []byte(token)) == 1 {
			return u, true
		}
	}
	return User{}, false
}

func (s *Store) Put(p *Provider) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.Id == "" {
		return errors.New("empty provider id")
	}
	p.Streams = normalizeStreams(p.Streams, p)
	if _, ok := s.providers[p.Id]; !ok {
		s.order = append(s.order, p.Id)
		sort.Strings(s.order)
	}
	s.providers[p.Id] = p
	return s.writeProviderLocked(p)
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.providers[id]; !ok {
		return fmt.Errorf("unknown provider %q", id)
	}
	delete(s.providers, id)
	kept := s.order[:0]
	for _, o := range s.order {
		if o != id {
			kept = append(kept, o)
		}
	}
	s.order = kept
	path := filepath.Join(s.dir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Store) writeProviderLocked(p *Provider) error {
	raw, err := json.MarshalIndent(p, "", "\t")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	target := filepath.Join(s.dir, p.Id+".json")
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

func (s *Store) SaveUsers() error {
	s.mu.RLock()
	raw, err := json.MarshalIndent(Config{Users: s.users, NoAuth: s.noAuth}, "", "\t")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	if s.confPath == "" {
		return nil
	}
	raw = append(raw, '\n')
	tmp := s.confPath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.confPath)
}

func slug(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
		case r == ' ' || r == '_' || r == '-':
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "stream"
	}
	return string(out)
}

func Slug(name string) string {
	return slug(name)
}
