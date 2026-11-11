package config

import "testing"

func TestEnsureDefaultAdminEmpty(t *testing.T) {
	users, changed := EnsureDefaultAdmin(nil)
	if !changed || len(users) != 1 {
		t.Fatalf("got %+v changed=%v", users, changed)
	}
	if users[0].Username != "admin" || users[0].Password != "admin" || !users[0].IsAdmin || !users[0].HasWebAccess {
		t.Fatalf("admin: %+v", users[0])
	}
	if users[0].StreamToken == "" {
		t.Fatal("missing stream token")
	}
}

func TestEnsureDefaultAdminSkipsWhenPasswordExists(t *testing.T) {
	in := []User{{Username: "bob", Password: "secret"}}
	out, changed := EnsureDefaultAdmin(in)
	if changed || len(out) != 1 || out[0].Username != "bob" {
		t.Fatalf("got %+v changed=%v", out, changed)
	}
}

func TestEnsureDefaultAdminFillsExistingAdmin(t *testing.T) {
	in := []User{{Username: "admin", Password: "", HasWebAccess: false}}
	out, changed := EnsureDefaultAdmin(in)
	if !changed || len(out) != 1 {
		t.Fatalf("got %+v changed=%v", out, changed)
	}
	if out[0].Password != "admin" || !out[0].IsAdmin || !out[0].HasWebAccess || out[0].StreamToken == "" {
		t.Fatalf("admin: %+v", out[0])
	}
}
