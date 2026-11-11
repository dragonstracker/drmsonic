package web

import (
	"encoding/json"
	"errors"
	"sort"

	"drmsonic/internal/config"
)

func importBody(raw map[string]any) (map[string]any, error) {
	if m, ok := raw["Body"].(map[string]any); ok {
		return m, nil
	}
	if m, ok := raw["Provider"].(map[string]any); ok {
		return m, nil
	}
	cfg := mapGet(raw, "ProviderImportConfig")
	switch v := cfg.(type) {
	case map[string]any:
		return v, nil
	case string:
		var m map[string]any
		if err := json.Unmarshal([]byte(v), &m); err != nil {
			return nil, errors.New("provider file is not valid JSON")
		}
		return m, nil
	}
	return nil, errors.New("import needs a provider file")
}

func providerFromBody(body map[string]any) (*config.Provider, error) {
	if _, ok := body["Streams"]; !ok {
		if channels, ok := body["Channels"]; ok {
			body["Streams"] = channels
			delete(body, "Channels")
		}
	}
	if v, ok := body["XForwardedFor"].(string); ok {
		body["XForwardedFor"] = v == "1" || v == "true"
	}
	if list, ok := body["Streams"].([]any); ok {
		for _, e := range list {
			st, ok := e.(map[string]any)
			if !ok {
				continue
			}
			if _, ok := st["Manifest"]; !ok {
				if u, ok := st["ManifestUrl"].(string); ok {
					st["Manifest"] = u
				}
			}
			switch h := st["Headers"].(type) {
			case nil:
				delete(st, "Headers")
			case map[string]any:
				if lines := headerLines(h); len(lines) > 0 {
					st["Headers"] = lines
				} else {
					delete(st, "Headers")
				}
			}
		}
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	p := &config.Provider{}
	if err := json.Unmarshal(raw, p); err != nil {
		return nil, errors.New("provider file does not match the provider schema")
	}
	return p, nil
}

func headerLines(h map[string]any) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, scope := range []string{"Manifest", "Media"} {
		block, ok := h[scope].(map[string]any)
		if !ok {
			continue
		}
		keys := make([]string, 0, len(block))
		for k := range block {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, k+": "+str(block[k]))
		}
	}
	return out
}
