package gomatrixserverlib

import (
	"errors"
	"testing"
)

func TestEventIDHookOverridesReferenceHash(t *testing.T) {
	const v = RoomVersionV10
	json := []byte(`{"type":"m.room.message","room_id":"!r:x","sender":"@a:x","content":{},"origin_server_ts":1,"depth":1,"prev_events":[],"auth_events":[]}`)
	want, err := referenceOfEvent(json, v)
	if err != nil {
		t.Fatal(err)
	}

	SetEventIDHook(func(RoomVersion, []byte) (string, error) { return want.EventID, nil })
	defer SetEventIDHook(nil)
	got, err := referenceOfEvent(json, v)
	if err != nil || got.EventID != want.EventID || string(got.EventSHA256) != string(want.EventSHA256) {
		t.Fatalf("hook result differs: %+v %v vs %+v", got, err, want)
	}

	boom := errors.New("boom")
	SetEventIDHook(func(RoomVersion, []byte) (string, error) { return "", boom })
	if _, err := referenceOfEvent(json, v); !errors.Is(err, boom) {
		t.Fatalf("hook error not propagated: %v", err)
	}
}

func TestContentHashHookIsUsed(t *testing.T) {
	boom := errors.New("boom")
	SetContentHashHook(func(RoomVersion, []byte) error { return boom })
	defer SetContentHashHook(nil)
	if err := checkContentHash([]byte(`{}`), RoomVersionV10); !errors.Is(err, boom) {
		t.Fatalf("hook not used: %v", err)
	}
}
