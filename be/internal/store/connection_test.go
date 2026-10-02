package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectionRoundTripAndSecretIsolation(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	c, err := s.CreateConnection("groq", "work key", "gsk-secret-value")
	if err != nil {
		t.Fatalf("CreateConnection: %v", err)
	}
	if c.ID == "" {
		t.Fatal("CreateConnection returned an empty id")
	}

	list, err := s.ListConnections()
	if err != nil {
		t.Fatalf("ListConnections: %v", err)
	}
	if len(list) != 1 || list[0].Provider != "groq" {
		t.Fatalf("ListConnections = %+v, want one groq row", list)
	}

	// The secret must not be reachable through the listing type, even by
	// serializing it. This is the guard the design calls for.
	blob, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), "gsk-secret-value") {
		t.Errorf("secret leaked through the listing: %s", blob)
	}

	got, err := s.Secret(c.ID)
	if err != nil {
		t.Fatalf("Secret: %v", err)
	}
	if got != "gsk-secret-value" {
		t.Errorf("Secret = %q, want the stored value", got)
	}

	if _, err := s.Secret("no-such-id"); err == nil {
		t.Error("Secret on an unknown id returned no error")
	}
}

func TestDeleteConnection(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c, _ := s.CreateConnection("groq", "temp", "secret")
	if err := s.DeleteConnection(c.ID); err != nil {
		t.Fatalf("DeleteConnection: %v", err)
	}
	list, _ := s.ListConnections()
	if len(list) != 0 {
		t.Errorf("connection still present after delete: %+v", list)
	}
	// deleting an unknown id is reported, not silent.
	if err := s.DeleteConnection("no-such-id"); err == nil {
		t.Error("DeleteConnection on unknown id returned no error")
	}
}

func TestSeedKeylessConnectionsAddsZenOnceAndRespectsDeletion(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	count := func() (n int, id string) {
		list, err := s.ListConnections()
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range list {
			if c.Provider == "opencode" {
				n++
				id = c.ID
			}
		}
		return n, id
	}

	for i := 0; i < 2; i++ {
		if err := s.SeedKeylessConnections(); err != nil {
			t.Fatalf("SeedKeylessConnections: %v", err)
		}
	}
	n, id := count()
	if n != 1 {
		t.Fatalf("opencode connections = %d after two seeds, want 1", n)
	}

	// An owner who deletes the account keeps it deleted across restarts.
	if err := s.DeleteConnection(id); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedKeylessConnections(); err != nil {
		t.Fatal(err)
	}
	if n, _ := count(); n != 0 {
		t.Fatalf("opencode connections = %d after delete and reseed, want 0", n)
	}
}

func TestSeedKeylessConnectionsKeepsAnExistingAccount(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.CreateConnection("opencode", "OpenCode Zen", "public"); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedKeylessConnections(); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListConnections()
	if len(list) != 1 {
		t.Fatalf("connections = %d, want the existing one only", len(list))
	}
}
