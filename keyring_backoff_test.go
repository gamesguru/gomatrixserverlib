package gomatrixserverlib

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gamesguru/gomatrixserverlib/spec"
)

type failingKeyClient struct{ calls atomic.Int32 }

func (c *failingKeyClient) GetServerKeys(context.Context, spec.ServerName) (ServerKeys, error) {
	c.calls.Add(1)
	return ServerKeys{}, errors.New("unreachable")
}

func (c *failingKeyClient) LookupServerKeys(context.Context, spec.ServerName, map[PublicKeyLookupRequest]spec.Timestamp) ([]ServerKeys, error) {
	c.calls.Add(1)
	return nil, errors.New("unreachable")
}

func TestDirectKeyFetcherBacksOffFailedServers(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	client := &failingKeyClient{}
	fetcher := &DirectKeyFetcher{
		Client:            client,
		IsLocalServerName: func(spec.ServerName) bool { return false },
		Backoff:           &KeyFetchBackoff{now: func() time.Time { return now }},
	}
	req := map[PublicKeyLookupRequest]spec.Timestamp{{ServerName: "dead.example", KeyID: "ed25519:a"}: 1}
	fetch := func() int32 {
		before := client.calls.Load()
		if _, err := fetcher.FetchKeys(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		return client.calls.Load() - before
	}

	if got := fetch(); got != 2 { // direct, then notary
		t.Fatalf("first fetch made %d calls, want 2", got)
	}
	if got := fetch(); got != 0 {
		t.Fatalf("fetch during backoff made %d calls, want 0", got)
	}
	now = now.Add(keyFetchBackoffBase + time.Second)
	if got := fetch(); got != 2 {
		t.Fatalf("fetch after backoff made %d calls, want 2", got)
	}
	// The second failure doubles the wait.
	now = now.Add(keyFetchBackoffBase + time.Second)
	if got := fetch(); got != 0 {
		t.Fatalf("fetch inside the doubled backoff made %d calls, want 0", got)
	}
	now = now.Add(keyFetchBackoffBase)
	if got := fetch(); got != 2 {
		t.Fatalf("fetch after the doubled backoff made %d calls, want 2", got)
	}
}

func TestKeyFetchBackoffSuccessClearsAndCaps(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	b := &KeyFetchBackoff{now: func() time.Time { return now }}
	for range 40 {
		b.Failure("a.example")
	}
	if !b.BackingOff("a.example") {
		t.Fatal("expected backoff")
	}
	now = now.Add(keyFetchBackoffMax + time.Second)
	if b.BackingOff("a.example") {
		t.Fatal("backoff exceeded the cap")
	}
	b.Failure("a.example")
	b.Success("a.example")
	if b.BackingOff("a.example") {
		t.Fatal("success did not clear the backoff")
	}
}
