package gomatrixserverlib

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"golang.org/x/crypto/ed25519"

	"codefloe.com/pat-s/gomatrixserverlib/spec"
)

type inviteStateFixture struct {
	keyID    KeyID
	sk       ed25519.PrivateKey
	otherSK  ed25519.PrivateKey
	verifier JSONVerifier
	user     spec.UserID
}

func newInviteStateFixture(t *testing.T) inviteStateFixture {
	t.Helper()
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, otherSK, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	user, err := spec.NewUserID("@user:server", true)
	require.NoError(t, err)
	return inviteStateFixture{
		keyID:    KeyID("ed25519:1234"),
		sk:       sk,
		otherSK:  otherSK,
		verifier: &KeyRing{[]KeyFetcher{&TestRequestKeyDummy{}}, &joinKeyDatabase{key: pk}},
		user:     *user,
	}
}

func (f inviteStateFixture) stateEvent(t *testing.T, version RoomVersion, roomID, eventType string, content string, sk ed25519.PrivateKey) PDU {
	t.Helper()
	stateKey := ""
	ev, err := MustGetRoomVersion(version).NewEventBuilderFromProtoEvent(&ProtoEvent{
		SenderID:   f.user.String(),
		RoomID:     roomID,
		Type:       eventType,
		StateKey:   &stateKey,
		PrevEvents: []any{},
		AuthEvents: []any{},
		Content:    spec.RawJSON(content),
	}).Build(time.Now(), f.user.Domain(), f.keyID, sk)
	require.NoError(t, err)
	return ev
}

// strippedOnly returns entries in the pre-MSC4311 stripped form.
func strippedOnly(t *testing.T, events ...PDU) FederationStrippedState {
	t.Helper()
	state := make(FederationStrippedState, len(events))
	for i, ev := range events {
		state[i] = NewInviteStrippedState(ev)
		state[i].pduJSON = nil
	}
	return state
}

// receivedState round-trips entries through JSON the way a federation
// request would deliver them.
func receivedState(t *testing.T, state FederationStrippedState) []InviteStrippedState {
	t.Helper()
	raw, err := json.Marshal(state)
	require.NoError(t, err)
	var received FederationStrippedState
	require.NoError(t, json.Unmarshal(raw, &received))
	return received
}

func TestInviteStrippedStateMarshalling(t *testing.T) {
	f := newInviteStateFixture(t)
	create := f.stateEvent(t, RoomVersionV10, "!room:server", spec.MRoomCreate, `{"room_version":"10"}`, f.sk)
	name := f.stateEvent(t, RoomVersionV10, "!room:server", spec.MRoomName, `{"name":"Test"}`, f.sk)

	fromPDU := NewInviteStrippedState(create)
	var strippedOnly InviteStrippedState
	require.NoError(t, json.Unmarshal([]byte(`{"type":"m.room.topic","state_key":"","sender":"@user:server","content":{"topic":"x"}}`), &strippedOnly))
	strippedOnly.pduJSON = nil

	// The default representation, used for clients, must never leak PDU fields.
	clientJSON, err := json.Marshal([]InviteStrippedState{fromPDU})
	require.NoError(t, err)
	for _, field := range []string{"event_id", "signatures", "hashes", "room_id", "origin_server_ts"} {
		assert.False(t, gjson.GetBytes(clientJSON, "0."+field).Exists(), "client JSON leaked %s", field)
	}
	assert.Equal(t, spec.MRoomCreate, gjson.GetBytes(clientJSON, "0.type").Str)

	// Over federation, events are sent as full PDUs where known.
	fedJSON, err := json.Marshal(FederationStrippedState{fromPDU, NewInviteStrippedState(name), strippedOnly})
	require.NoError(t, err)
	assert.JSONEq(t, string(create.JSON()), gjson.GetBytes(fedJSON, "0").Raw)
	assert.JSONEq(t, string(name.JSON()), gjson.GetBytes(fedJSON, "1").Raw)
	assert.JSONEq(t, `{"type":"m.room.topic","state_key":"","sender":"@user:server","content":{"topic":"x"}}`, gjson.GetBytes(fedJSON, "2").Raw)

	nilJSON, err := json.Marshal(FederationStrippedState(nil))
	require.NoError(t, err)
	assert.Equal(t, "null", string(nilJSON))

	// Received entries keep their full JSON and still expose stripped fields.
	received := receivedState(t, FederationStrippedState{fromPDU})
	require.Len(t, received, 1)
	assert.JSONEq(t, string(create.JSON()), string(received[0].PDUJSON()))
	assert.Equal(t, spec.MRoomCreate, received[0].Type())
	assert.Equal(t, "@user:server", received[0].Sender())
}

func TestValidateStrippedStatePDUs(t *testing.T) {
	f := newInviteStateFixture(t)
	ctx := context.Background()

	v10Room, err := spec.NewRoomID("!room:server")
	require.NoError(t, err)
	v10Create := f.stateEvent(t, RoomVersionV10, v10Room.String(), spec.MRoomCreate, `{"room_version":"10"}`, f.sk)
	v10Name := f.stateEvent(t, RoomVersionV10, v10Room.String(), spec.MRoomName, `{"name":"Test"}`, f.sk)
	v10OtherRoomCreate := f.stateEvent(t, RoomVersionV10, "!other:server", spec.MRoomCreate, `{"room_version":"10"}`, f.sk)
	v10BadSigCreate := f.stateEvent(t, RoomVersionV10, v10Room.String(), spec.MRoomCreate, `{"room_version":"10"}`, f.otherSK)
	v10WrongVersionCreate := f.stateEvent(t, RoomVersionV10, v10Room.String(), spec.MRoomCreate, `{"room_version":"9"}`, f.sk)

	v12Create := f.stateEvent(t, RoomVersionV12, "", spec.MRoomCreate, `{"room_version":"12"}`, f.sk)
	v12Room := v12Create.RoomID()
	v12Name := f.stateEvent(t, RoomVersionV12, v12Room.String(), spec.MRoomName, `{"name":"Test"}`, f.sk)
	v12OtherCreate := f.stateEvent(t, RoomVersionV12, "", spec.MRoomCreate, `{"room_version":"12","other":true}`, f.sk)

	var strippedOnly InviteStrippedState
	require.NoError(t, json.Unmarshal([]byte(`{"type":"m.room.create","state_key":"","sender":"@user:server","content":{"room_version":"10"}}`), &strippedOnly))

	tests := map[string]struct {
		roomID        spec.RoomID
		version       RoomVersion
		state         []InviteStrippedState
		wantValid     int
		wantErr       bool
		missingCreate bool
	}{
		"v10_valid": {
			roomID: *v10Room, version: RoomVersionV10,
			state:     receivedState(t, FederationStrippedState{NewInviteStrippedState(v10Create), NewInviteStrippedState(v10Name)}),
			wantValid: 2,
		},
		"v10_empty": {
			roomID: *v10Room, version: RoomVersionV10,
			wantErr: true, missingCreate: true,
		},
		"v10_missing_create": {
			roomID: *v10Room, version: RoomVersionV10,
			state:     receivedState(t, FederationStrippedState{NewInviteStrippedState(v10Name)}),
			wantValid: 1, wantErr: true, missingCreate: true,
		},
		"v10_stripped_only": {
			roomID: *v10Room, version: RoomVersionV10,
			state:   []InviteStrippedState{strippedOnly},
			wantErr: true, missingCreate: true,
		},
		"v10_create_for_other_room": {
			roomID: *v10Room, version: RoomVersionV10,
			state:     receivedState(t, FederationStrippedState{NewInviteStrippedState(v10OtherRoomCreate), NewInviteStrippedState(v10Name)}),
			wantValid: 1, wantErr: true, missingCreate: true,
		},
		"v10_bad_signature": {
			roomID: *v10Room, version: RoomVersionV10,
			state:   receivedState(t, FederationStrippedState{NewInviteStrippedState(v10BadSigCreate)}),
			wantErr: true, missingCreate: true,
		},
		"v10_create_room_version_mismatch": {
			roomID: *v10Room, version: RoomVersionV10,
			state:   receivedState(t, FederationStrippedState{NewInviteStrippedState(v10WrongVersionCreate)}),
			wantErr: true, missingCreate: true,
		},
		"v12_valid": {
			roomID: v12Room, version: RoomVersionV12,
			state:     receivedState(t, FederationStrippedState{NewInviteStrippedState(v12Create), NewInviteStrippedState(v12Name)}),
			wantValid: 2,
		},
		"v12_create_hashes_to_other_room": {
			roomID: v12Room, version: RoomVersionV12,
			state:     receivedState(t, FederationStrippedState{NewInviteStrippedState(v12OtherCreate), NewInviteStrippedState(v12Name)}),
			wantValid: 1, wantErr: true, missingCreate: true,
		},
		"v12_create_with_foreign_room_id": {
			roomID: v12Room, version: RoomVersionV12,
			state:   receivedState(t, FederationStrippedState{NewInviteStrippedState(v10Create)}),
			wantErr: true, missingCreate: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			valid, err := ValidateStrippedStatePDUs(ctx, tc.roomID, tc.version, tc.state, f.verifier, UserIDForSenderTest)
			assert.Len(t, valid, tc.wantValid)
			if !tc.wantErr {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tc.missingCreate, errors.Is(err, ErrStrippedStateMissingCreate))
			for _, entry := range valid {
				assert.NotEmpty(t, entry.PDUJSON())
			}
		})
	}
}

func TestStrippedStateRejectsInvite(t *testing.T) {
	notPDU := StrippedStateEntryError{Index: 0, Err: ErrStrippedStateNotPDU}
	otherRoom := StrippedStateEntryError{Index: 1, Err: errors.New("event belongs to room \"!b\", not \"!a\"")}
	tests := map[string]struct {
		err  error
		want bool
	}{
		"no error":                       {err: nil, want: false},
		"missing create":                 {err: errors.Join(ErrStrippedStateMissingCreate), want: false},
		"stripped state only":            {err: errors.Join(notPDU, ErrStrippedStateMissingCreate), want: false},
		"PDU for another room":           {err: errors.Join(otherRoom, ErrStrippedStateMissingCreate), want: true},
		"PDU for another room, unjoined": {err: otherRoom, want: true},
		"mixed":                          {err: errors.Join(notPDU, otherRoom), want: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, StrippedStateRejectsInvite(RoomVersionV12, tc.err))
			// Room versions 1 through 11 never reject.
			for _, version := range []RoomVersion{RoomVersionV1, RoomVersionV10, RoomVersionV11, RoomVersionPseudoIDs} {
				assert.False(t, StrippedStateRejectsInvite(version, tc.err), version)
			}
		})
	}
	assert.False(t, StrippedStateRejectsInvite("unknown", otherRoom))
}

func TestHandleInviteValidatesInviteRoomState(t *testing.T) {
	f := newInviteStateFixture(t)
	ctx := context.Background()
	stateKey := f.user.String()

	v10Room, err := spec.NewRoomID("!room:server")
	require.NoError(t, err)
	v10Create := f.stateEvent(t, RoomVersionV10, v10Room.String(), spec.MRoomCreate, `{"room_version":"10"}`, f.sk)
	v10OtherRoomCreate := f.stateEvent(t, RoomVersionV10, "!other:server", spec.MRoomCreate, `{"room_version":"10"}`, f.sk)
	v10Invite, err := createMemberEventBuilder(RoomVersionV10, f.user.String(), v10Room.String(), &stateKey, spec.RawJSON(`{"membership":"invite"}`)).
		Build(time.Now(), f.user.Domain(), f.keyID, f.sk)
	require.NoError(t, err)

	v12Create := f.stateEvent(t, RoomVersionV12, "", spec.MRoomCreate, `{"room_version":"12"}`, f.sk)
	v12Room := v12Create.RoomID()
	v12Name := f.stateEvent(t, RoomVersionV12, v12Room.String(), spec.MRoomName, `{"name":"Test"}`, f.sk)
	v12OtherCreate := f.stateEvent(t, RoomVersionV12, "", spec.MRoomCreate, `{"room_version":"12","other":true}`, f.sk)
	v12BadSigName := f.stateEvent(t, RoomVersionV12, v12Room.String(), spec.MRoomName, `{"name":"Forged"}`, f.otherSK)
	v12Invite, err := createMemberEventBuilder(RoomVersionV12, f.user.String(), v12Room.String(), &stateKey, spec.RawJSON(`{"membership":"invite"}`)).
		Build(time.Now(), f.user.Domain(), f.keyID, f.sk)
	require.NoError(t, err)

	input := func(roomID spec.RoomID, version RoomVersion, invite PDU, state FederationStrippedState) HandleInviteInput {
		return HandleInviteInput{
			RoomID:            roomID,
			RoomVersion:       version,
			InvitedUser:       f.user,
			InviteEvent:       invite,
			StrippedState:     receivedState(t, state),
			RoomQuerier:       &TestRoomQuerier{},
			MembershipQuerier: &TestMembershipQuerier{},
			StateQuerier:      &TestStateQuerier{},
			KeyID:             f.keyID,
			PrivateKey:        f.sk,
			Verifier:          f.verifier,
			UserIDQuerier:     UserIDForSenderTest,
		}
	}

	tests := map[string]struct {
		input          HandleInviteInput
		wantMissingErr bool
	}{
		"v12_valid_state": {
			input: input(v12Room, RoomVersionV12, v12Invite, FederationStrippedState{NewInviteStrippedState(v12Create), NewInviteStrippedState(v12Name)}),
		},
		// MSC4311's migration section: warn about state which is missing or
		// not in PDU form, but fail on PDUs which do not check out.
		"v12_no_state_is_accepted": {
			input: input(v12Room, RoomVersionV12, v12Invite, nil),
		},
		"v12_missing_create_is_accepted": {
			input: input(v12Room, RoomVersionV12, v12Invite, FederationStrippedState{NewInviteStrippedState(v12Name)}),
		},
		"v12_stripped_state_is_accepted": {
			input: input(v12Room, RoomVersionV12, v12Invite, strippedOnly(t, v12Create, v12Name)),
		},
		"v12_create_for_other_room": {
			input:          input(v12Room, RoomVersionV12, v12Invite, FederationStrippedState{NewInviteStrippedState(v12OtherCreate)}),
			wantMissingErr: true,
		},
		"v12_bad_signature": {
			input:          input(v12Room, RoomVersionV12, v12Invite, FederationStrippedState{NewInviteStrippedState(v12Create), NewInviteStrippedState(v12BadSigName)}),
			wantMissingErr: true,
		},
		"v10_valid_state": {
			input: input(*v10Room, RoomVersionV10, v10Invite, FederationStrippedState{NewInviteStrippedState(v10Create)}),
		},
		// Room versions 1 through 11 only warn, so older servers can still invite.
		"v10_create_for_other_room_is_accepted": {
			input: input(*v10Room, RoomVersionV10, v10Invite, FederationStrippedState{NewInviteStrippedState(v10OtherRoomCreate)}),
		},
		"v10_no_state_is_accepted": {
			input: input(*v10Room, RoomVersionV10, v10Invite, nil),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			event, err := HandleInvite(ctx, tc.input)
			if tc.wantMissingErr {
				var matrixErr spec.MatrixError
				require.ErrorAs(t, err, &matrixErr)
				assert.Equal(t, spec.ErrorMissingParam, matrixErr.ErrCode)
				return
			}
			require.NoError(t, err)
			// Clients only ever see the stripped form of the received state.
			inviteState := gjson.GetBytes(event.Unsigned(), "invite_room_state")
			for _, entry := range inviteState.Array() {
				assert.False(t, entry.Get("signatures").Exists(), "invite_room_state leaked PDU fields: %s", entry.Raw)
				assert.False(t, entry.Get("event_id").Exists(), "invite_room_state leaked PDU fields: %s", entry.Raw)
			}
			if len(tc.input.StrippedState) > 0 {
				assert.Len(t, inviteState.Array(), len(tc.input.StrippedState))
			}
		})
	}
}

func TestRemoteInviteError(t *testing.T) {
	missingParam := spec.HTTPError{
		Code:         400,
		WrappedError: spec.RespError{ErrCode: string(spec.ErrorMissingParam), Err: "Create event not among invite state entries."},
	}
	err := remoteInviteError(errors.Join(errors.New("r.federation.SendInviteV2"), missingParam))
	assert.ErrorIs(t, err, ErrRemoteRejectedInviteState)
	var matrixErr spec.MatrixError
	assert.False(t, errors.As(err, &matrixErr), "must not surface as a Matrix error, got %v", err)

	forbidden := spec.HTTPError{
		Code:         403,
		WrappedError: spec.RespError{ErrCode: string(spec.ErrorForbidden), Err: "no"},
	}
	for _, other := range []error{forbidden, errors.New("connection refused")} {
		err = remoteInviteError(other)
		require.ErrorAs(t, err, &matrixErr)
		assert.Equal(t, spec.ErrorForbidden, matrixErr.ErrCode)
	}
}

func TestGenerateStrippedStateKeepsPDUs(t *testing.T) {
	f := newInviteStateFixture(t)
	create := f.stateEvent(t, RoomVersionV12, "", spec.MRoomCreate, `{"room_version":"12"}`, f.sk)
	roomID := create.RoomID()
	name := f.stateEvent(t, RoomVersionV12, roomID.String(), spec.MRoomName, `{"name":"Test"}`, f.sk)

	state, err := GenerateStrippedState(context.Background(), roomID, &TestStateQuerier{state: []PDU{create, name}})
	require.NoError(t, err)
	require.Len(t, state, 2)

	// Our own invites must pass the validation remote servers apply to them.
	valid, err := ValidateStrippedStatePDUs(context.Background(), roomID, RoomVersionV12, receivedState(t, state), f.verifier, UserIDForSenderTest)
	require.NoError(t, err)
	assert.Len(t, valid, 2)
}
