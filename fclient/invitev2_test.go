package fclient

import (
	"encoding/json"
	"testing"

	"codefloe.com/pat-s/gomatrixserverlib"
)

const TestInviteV2ExampleEvent = `{"_room_version":"1","auth_events":[["$oXL79cT7fFxR7dPH:localhost",{"sha256":"abjkiDSg1RkuZrbj2jZoGMlQaaj1Ue3Jhi7I7NlKfXY"}],["$IVUsaSkm1LBAZYYh:localhost",{"sha256":"X7RUj46hM/8sUHNBIFkStbOauPvbDzjSdH4NibYWnko"}],["$VS2QT0EeArZYi8wf:localhost",{"sha256":"k9eM6utkCH8vhLW9/oRsH74jOBS/6RVK42iGDFbylno"}]],"content":{"name":"test3"},"depth":7,"event_id":"$yvN1b43rlmcOs5fY:localhost","hashes":{"sha256":"Oh1mwI1jEqZ3tgJ+V1Dmu5nOEGpCE4RFUqyJv2gQXKs"},"origin":"localhost","origin_server_ts":1510854416361,"prev_events":[["$FqI6TVvWpcbcnJ97:localhost",{"sha256":"upCsBqUhNUgT2/+zkzg8TbqdQpWWKQnZpGJc6KcbUC4"}]],"prev_state":[],"room_id":"!19Mp0U9hjajeIiw1:localhost","sender":"@test:localhost","signatures":{"localhost":{"ed25519:u9kP":"5IzSuRXkxvbTp0vZhhXYZeOe+619iG3AybJXr7zfNn/4vHz4TH7qSJVQXSaHHvcTcDodAKHnTG1WDulgO5okAQ"}},"state_key":"","type":"m.room.name"}`

func TestMarshalInviteV2Request(t *testing.T) {
	expected := `{"room_version":"1","invite_room_state":[],"event":{"auth_events":[["$oXL79cT7fFxR7dPH:localhost",{"sha256":"abjkiDSg1RkuZrbj2jZoGMlQaaj1Ue3Jhi7I7NlKfXY"}],["$IVUsaSkm1LBAZYYh:localhost",{"sha256":"X7RUj46hM/8sUHNBIFkStbOauPvbDzjSdH4NibYWnko"}],["$VS2QT0EeArZYi8wf:localhost",{"sha256":"k9eM6utkCH8vhLW9/oRsH74jOBS/6RVK42iGDFbylno"}]],"content":{"name":"test3"},"depth":7,"event_id":"$yvN1b43rlmcOs5fY:localhost","hashes":{"sha256":"Oh1mwI1jEqZ3tgJ+V1Dmu5nOEGpCE4RFUqyJv2gQXKs"},"origin":"localhost","origin_server_ts":1510854416361,"prev_events":[["$FqI6TVvWpcbcnJ97:localhost",{"sha256":"upCsBqUhNUgT2/+zkzg8TbqdQpWWKQnZpGJc6KcbUC4"}]],"prev_state":[],"room_id":"!19Mp0U9hjajeIiw1:localhost","sender":"@test:localhost","signatures":{"localhost":{"ed25519:u9kP":"5IzSuRXkxvbTp0vZhhXYZeOe+619iG3AybJXr7zfNn/4vHz4TH7qSJVQXSaHHvcTcDodAKHnTG1WDulgO5okAQ"}},"state_key":"","type":"m.room.name"}}`

	output, err := gomatrixserverlib.NewEventFromHeaderedJSON([]byte(TestInviteV2ExampleEvent), false)
	if err != nil {
		t.Fatal(err)
	}

	inviteReq, err := NewInviteV2Request(output, []gomatrixserverlib.InviteStrippedState{})
	if err != nil {
		t.Fatal(err)
	}

	j, err := json.Marshal(inviteReq)
	if err != nil {
		t.Fatal(err)
	}

	if string(j) != expected {
		t.Fatalf("got %q, expected %q", string(j), expected)
	}
}

func TestStrippedState(t *testing.T) {
	expected := `{"content":{"name":"test3"},"state_key":"","type":"m.room.name","sender":"@test:localhost"}`

	output, err := gomatrixserverlib.NewEventFromHeaderedJSON([]byte(TestInviteV2ExampleEvent), false)
	if err != nil {
		t.Fatal(err)
	}

	stripped := gomatrixserverlib.NewInviteStrippedState(output)

	j, err := json.Marshal(stripped)
	if err != nil {
		t.Fatal(err)
	}

	if string(j) != expected {
		t.Fatalf("got %q, expected %q", string(j), expected)
	}
}

func TestInviteV2RequestSendsRoomStateAsPDUs(t *testing.T) {
	output, err := gomatrixserverlib.NewEventFromHeaderedJSON([]byte(TestInviteV2ExampleEvent), false)
	if err != nil {
		t.Fatal(err)
	}

	inviteReq, err := NewInviteV2Request(output, []gomatrixserverlib.InviteStrippedState{
		gomatrixserverlib.NewInviteStrippedState(output),
	})
	if err != nil {
		t.Fatal(err)
	}

	j, err := json.Marshal(inviteReq)
	if err != nil {
		t.Fatal(err)
	}

	var wire struct {
		InviteRoomState []json.RawMessage `json:"invite_room_state"`
	}
	if err = json.Unmarshal(j, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.InviteRoomState) != 1 || string(wire.InviteRoomState[0]) != string(output.JSON()) {
		t.Fatalf("invite_room_state should hold the full PDU, got %s", j)
	}

	var received InviteV2Request
	if err = json.Unmarshal(j, &received); err != nil {
		t.Fatal(err)
	}
	state := received.InviteRoomState()
	if len(state) != 1 || string(state[0].PDUJSON()) != string(output.JSON()) {
		t.Fatalf("received invite_room_state should keep the full PDU, got %+v", state)
	}
	if state[0].Type() != "m.room.name" {
		t.Fatalf("unexpected stripped type %q", state[0].Type())
	}
}
