package fclient

import (
	"crypto/ed25519"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/google/go-cmp/cmp"

	"github.com/gamesguru/gomatrixserverlib"
	"github.com/gamesguru/gomatrixserverlib/spec"
)

const (
	emptyRespStateResponse    = `{"pdus":[],"auth_chain":[]}`
	emptyRespSendJoinResponse = `{"state":[],"auth_chain":[],"origin":""}`
)

func TestParseServerName(t *testing.T) {
	validTests := map[string][]any{
		"www.example.org:1234":         {"www.example.org", 1234},
		"www.example.org":              {"www.example.org", -1},
		"1234.example.com":             {"1234.example.com", -1},
		"1.1.1.1:1234":                 {"1.1.1.1", 1234},
		"1.1.1.1":                      {"1.1.1.1", -1},
		"[1fff:0:a88:85a3::ac1f]:1234": {"[1fff:0:a88:85a3::ac1f]", 1234},
		"[2001:0db8::ff00:0042]":       {"[2001:0db8::ff00:0042]", -1},
	}

	for input, output := range validTests {
		host, port, isValid := spec.ParseAndValidateServerName(spec.ServerName(input))
		if !isValid {
			t.Errorf("Expected serverName '%s' to be parsed as valid, but was not", input)
		}

		expectedPort, ok := output[1].(int)
		if !ok {
			t.Fatalf("Expected port for '%s' to be int, but was %T", input, output[1])
		}
		if host != output[0] || port != expectedPort {
			t.Errorf(
				"Expected serverName '%s' to be cleaned and validated to '%s', %d, got '%s', %d",
				input, output[0], output[1], host, port,
			)
		}
	}

	invalidTests := []string{
		// ipv6 not in square brackets
		"2001:0db8::ff00:0042",

		// host with invalid characters
		"test_test.com",

		// ipv6 with insufficient parts
		"[2001:0db8:0000:0000:0000:ff00:0042]",

		// empty host
		":8080",

		// empty string
		"",
	}

	for _, input := range invalidTests {
		_, _, isValid := spec.ParseAndValidateServerName(spec.ServerName(input))
		if isValid {
			t.Errorf("Expected serverName '%s' to be rejected but was accepted", input)
		}
	}
}

func TestRespStateMarshalJSON(t *testing.T) {
	inputData := `{"pdus":[],"auth_chain":[]}`
	var input RespState
	if err := json.Unmarshal([]byte(inputData), &input); err != nil {
		t.Fatal(err)
	}

	gotBytes, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}

	got := string(gotBytes)

	if emptyRespStateResponse != got {
		t.Errorf("json.Marshal(RespState(%q)): wanted %q, got %q", inputData, emptyRespStateResponse, got)
	}
}

func TestRespStateUnmarshalJSON(t *testing.T) {
	inputData := `{"pdus":[],"auth_chain":[]}`
	var input RespState
	if err := json.Unmarshal([]byte(inputData), &input); err != nil {
		t.Fatal(err)
	}

	gotBytes, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	got := string(gotBytes)

	if emptyRespStateResponse != got {
		t.Errorf("json.Marshal(RespSendJoin(%q)): wanted %q, got %q", inputData, emptyRespStateResponse, got)
	}
}

func TestRespSendJoinMarshalJSON(t *testing.T) {
	// we unmarshall and marshall an empty send-join response, and check it round-trips correctly.
	inputData := `{"state":[],"auth_chain":[],"origin":""}`
	var input RespSendJoin
	if err := json.Unmarshal([]byte(inputData), &input); err != nil {
		t.Fatal(err)
	}

	want := RespSendJoin{
		StateEvents: []spec.RawJSON{},
		AuthEvents:  []spec.RawJSON{},
		Origin:      "",
	}
	if !cmp.Equal(input, want, cmp.AllowUnexported(RespSendJoin{})) {
		t.Errorf("json.Unmarshal(%s): wanted %+v, got %+v", inputData, want, input)
	}

	gotBytes, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	got := string(gotBytes)
	if emptyRespSendJoinResponse != got {
		t.Errorf("json.Marshal(%+v): wanted '%s', got '%s'", input, emptyRespSendJoinResponse, got)
	}
}

func TestRespSendJoinMarshalJSONPartialState(t *testing.T) {
	inputData := `{
		"state":[],"auth_chain":[],"origin":"o1",
		"members_omitted":true,
		"servers_in_room":["s1", "s2"]
	}`

	var input RespSendJoin
	if err := json.Unmarshal([]byte(inputData), &input); err != nil {
		t.Fatal(err)
	}

	want := RespSendJoin{
		StateEvents:    []spec.RawJSON{},
		AuthEvents:     []spec.RawJSON{},
		Origin:         "o1",
		MembersOmitted: true,
		ServersInRoom:  []string{"s1", "s2"},
	}
	if !cmp.Equal(input, want, cmp.AllowUnexported(RespSendJoin{})) {
		t.Errorf("json.Unmarshal(%s): wanted %+v, got %+v", inputData, want, input)
	}

	gotBytes, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	got := string(gotBytes)
	// the result should be the input, with spaces removed
	wantJSON := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, inputData)
	if wantJSON != got {
		t.Errorf("json.Marshal(%+v):\n  wanted: '%s'\n     got: '%s'", input, wantJSON, got)
	}
}

func TestRespPeekUnmarshalJSON(t *testing.T) {
	_, sk, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	emptyStateKey := ""
	build := func(t *testing.T, version gomatrixserverlib.RoomVersion, proto gomatrixserverlib.ProtoEvent) gomatrixserverlib.PDU {
		t.Helper()
		ev, buildErr := gomatrixserverlib.MustGetRoomVersion(version).NewEventBuilderFromProtoEvent(&proto).
			Build(time.Now(), "localhost", "ed25519:1", sk)
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		return ev
	}
	latestEvent := func(t *testing.T, version gomatrixserverlib.RoomVersion) gomatrixserverlib.PDU {
		t.Helper()
		roomID := "!room:localhost"
		if version == gomatrixserverlib.RoomVersionV12 {
			create := build(t, version, gomatrixserverlib.ProtoEvent{
				Type: spec.MRoomCreate, StateKey: &emptyStateKey, SenderID: "@alice:localhost", Depth: 1,
				Content: spec.RawJSON(`{"room_version":"12"}`),
			})
			roomID = create.RoomID().String()
		}
		return build(t, version, gomatrixserverlib.ProtoEvent{
			Type: "m.room.message", RoomID: roomID, SenderID: "@alice:localhost", Depth: 2,
			Content: spec.RawJSON(`{"body":"<b>peeked</b>","msgtype":"m.text"}`),
		})
	}

	for _, version := range []gomatrixserverlib.RoomVersion{gomatrixserverlib.RoomVersionV10, gomatrixserverlib.RoomVersionV11, gomatrixserverlib.RoomVersionV12} {
		t.Run("room version "+string(version), func(t *testing.T) {
			want := latestEvent(t, version)
			data, marshalErr := json.Marshal(RespPeek{RenewalInterval: 3600000, RoomVersion: version, LatestEvent: want})
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			var got RespPeek
			if err = json.Unmarshal(data, &got); err != nil {
				t.Fatalf("json.Unmarshal(%s): %v", data, err)
			}
			if got.RenewalInterval != 3600000 || got.RoomVersion != version {
				t.Errorf("got renewal interval %d and room version %q", got.RenewalInterval, got.RoomVersion)
			}
			if got.LatestEvent == nil || got.LatestEvent.EventID() != want.EventID() || got.LatestEvent.Version() != version {
				t.Fatalf("latest event was not decoded as %s from %s", want.EventID(), data)
			}
			// json.Marshal escapes HTML, so compare the content as values.
			var wantContent, gotContent map[string]any
			if err = json.Unmarshal(want.Content(), &wantContent); err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(got.LatestEvent.Content(), &gotContent); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(wantContent, gotContent); diff != "" {
				t.Errorf("latest event content differs (-want +got):\n%s", diff)
			}
			if len(got.StateEvents) != 0 || len(got.AuthEvents) != 0 {
				t.Errorf("expected empty state and auth chain, got %d and %d", len(got.StateEvents), len(got.AuthEvents))
			}
		})
	}

	t.Run("without a latest event", func(t *testing.T) {
		var got RespPeek
		if err = json.Unmarshal([]byte(`{"renewal_interval":1,"state":[],"auth_chain":[],"room_version":"11","latest_event":null}`), &got); err != nil {
			t.Fatal(err)
		}
		if got.LatestEvent != nil {
			t.Errorf("expected no latest event, got %v", got.LatestEvent)
		}
	})

	t.Run("with an unknown room version", func(t *testing.T) {
		var got RespPeek
		err = json.Unmarshal([]byte(`{"room_version":"unknown","latest_event":{"type":"m.room.message"}}`), &got)
		if err == nil || !strings.Contains(err.Error(), "unsupported room version") {
			t.Errorf("expected an unsupported room version error, got %v", err)
		}
	})

	t.Run("with a malformed latest event", func(t *testing.T) {
		var got RespPeek
		err = json.Unmarshal([]byte(`{"room_version":"11","latest_event":{"type":"m.room.message","depth":1.5}}`), &got)
		if err == nil || !strings.Contains(err.Error(), "invalid latest_event") {
			t.Errorf("expected an invalid latest_event error, got %v", err)
		}
	})
}
