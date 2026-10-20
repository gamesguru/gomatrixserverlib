// Copyright 2023 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gomatrixserverlib

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/gamesguru/gomatrixserverlib/internal"
	"github.com/gamesguru/gomatrixserverlib/spec"
)

type RoomQuerier interface {
	IsKnownRoom(ctx context.Context, roomID spec.RoomID) (bool, error)
}

type StateQuerier interface {
	GetAuthEvents(ctx context.Context, event PDU) (AuthEventProvider, error)
	GetState(ctx context.Context, roomID spec.RoomID, stateWanted []StateKeyTuple) ([]PDU, error)
}

type LatestEvents struct {
	RoomExists   bool
	StateEvents  []PDU
	PrevEventIDs []string
	Depth        int64
}

type FederatedInviteClient interface {
	SendInvite(ctx context.Context, event PDU, strippedState []InviteStrippedState) (PDU, error)
	SendInviteV3(ctx context.Context, event ProtoEvent, userID spec.UserID, roomVersion RoomVersion, strippedState []InviteStrippedState) (PDU, error)
}

// InviteStrippedState is a cut-down set of fields from room state
// events that allow the invited server to identify the room.
//
// Since MSC4311 (Matrix 1.16), invite and knock room state travels over
// federation as full PDUs. An InviteStrippedState therefore also keeps the
// full event JSON it was built from or received as, see PDUJSON. MarshalJSON
// always emits the stripped form so that it is safe to hand to clients; use
// FederationStrippedState to marshal the PDUs for federation.
type InviteStrippedState struct {
	fields struct {
		Content  spec.RawJSON `json:"content"`
		StateKey *string      `json:"state_key"`
		Type     string       `json:"type"`
		SenderID string       `json:"sender"`
	}
	pduJSON spec.RawJSON
}

// NewInviteStrippedState creates a stripped state event from a
// regular state event.
func NewInviteStrippedState(event PDU) (ss InviteStrippedState) {
	ss.fields.Content = event.Content()
	ss.fields.StateKey = event.StateKey()
	ss.fields.Type = event.Type()
	ss.fields.SenderID = string(event.SenderID())
	ss.pduJSON = event.JSON()
	return
}

// MarshalJSON implements json.Marshaller.
// It emits only the stripped state fields, never the full PDU.
func (i InviteStrippedState) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.fields)
}

// UnmarshalJSON implements json.Unmarshaller.
// The raw entry is retained so that it can be validated as a PDU.
func (i *InviteStrippedState) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &i.fields); err != nil {
		return err
	}
	i.pduJSON = append(spec.RawJSON(nil), data...)
	return nil
}

// PDUJSON returns the full event JSON this entry was built from or received
// as. It is nil for entries which were not built from an event. Received
// entries are untrusted until they pass ValidateStrippedStatePDUs.
func (i *InviteStrippedState) PDUJSON() spec.RawJSON {
	return i.pduJSON
}

// FederationStrippedState is the federation representation of
// invite_room_state and knock_room_state. Entries are marshaled as full
// PDUs when the event JSON is known, as MSC4311 requires, and as stripped
// state otherwise.
type FederationStrippedState []InviteStrippedState

// MarshalJSON implements json.Marshaller.
func (s FederationStrippedState) MarshalJSON() ([]byte, error) {
	if s == nil {
		return []byte("null"), nil
	}
	entries := make([]json.RawMessage, len(s))
	for idx := range s {
		if len(s[idx].pduJSON) > 0 {
			entries[idx] = json.RawMessage(s[idx].pduJSON)
			continue
		}
		stripped, err := json.Marshal(s[idx])
		if err != nil {
			return nil, err
		}
		entries[idx] = stripped
	}
	return json.Marshal(entries)
}

// Content returns the content of the stripped state.
func (i *InviteStrippedState) Content() spec.RawJSON {
	return i.fields.Content
}

// StateKey returns the state key of the stripped state.
func (i *InviteStrippedState) StateKey() *string {
	return i.fields.StateKey
}

// Type returns the type of the stripped state.
func (i *InviteStrippedState) Type() string {
	return i.fields.Type
}

// Sender returns the sender of the stripped state.
func (i *InviteStrippedState) Sender() string {
	return i.fields.SenderID
}

func GenerateStrippedState(
	ctx context.Context, roomID spec.RoomID, stateQuerier StateQuerier,
) ([]InviteStrippedState, error) {
	// "If they are set on the room, at least the state for m.room.avatar, m.room.canonical_alias, m.room.join_rules, and m.room.name SHOULD be included."
	// https://matrix.org/docs/spec/client_server/r0.6.0#m-room-member
	stateWanted := []StateKeyTuple{}
	for _, t := range []string{
		spec.MRoomName, spec.MRoomCanonicalAlias,
		spec.MRoomJoinRules, spec.MRoomAvatar,
		spec.MRoomEncryption, spec.MRoomCreate,
	} {
		stateWanted = append(stateWanted, StateKeyTuple{
			EventType: t,
			StateKey:  "",
		})
	}

	stateEvents, err := stateQuerier.GetState(ctx, roomID, stateWanted)
	if err != nil {
		return []InviteStrippedState{}, err
	}
	if stateEvents != nil {
		inviteState := []InviteStrippedState{}
		for _, event := range stateEvents {
			inviteState = append(inviteState, NewInviteStrippedState(event))
		}
		return inviteState, nil
	}
	return []InviteStrippedState{}, nil
}

func abortIfAlreadyJoined(ctx context.Context, roomID spec.RoomID, invitedUser spec.SenderID, membershipQuerier MembershipQuerier) error {
	membership, err := membershipQuerier.CurrentMembership(ctx, roomID, invitedUser)
	if err != nil {
		internal.GetLogger(ctx).WithError(err).Error("failed getting user membership")
		return spec.InternalServerError{}
	}
	isAlreadyJoined := (membership == spec.Join)

	if isAlreadyJoined {
		// If the user is joined to the room then that takes precedence over this
		// invite event. It makes little sense to move a user that is already
		// joined to the room into the invite state.
		// This could plausibly happen if an invite request raced with a join
		// request for a user. For example if a user was invited to a public
		// room and they joined the room at the same time as the invite was sent.
		// The other way this could plausibly happen is if an invite raced with
		// a kick. For example if a user was kicked from a room in error and in
		// response someone else in the room re-invited them then it is possible
		// for the invite request to race with the leave event so that the
		// target receives invite before it learns that it has been kicked.
		// There are a few ways this could be plausibly handled in the roomserver.
		// 1) Store the invite, but mark it as retired. That will result in the
		//    permanent rejection of that invite event. So even if the target
		//    user leaves the room and the invite is retransmitted it will be
		//    ignored. However a new invite with a new event ID would still be
		//    accepted.
		// 2) Silently discard the invite event. This means that if the event
		//    was retransmitted at a later date after the target user had left
		//    the room we would accept the invite. However since we hadn't told
		//    the sending server that the invite had been discarded it would
		//    have no reason to attempt to retry.
		// 3) Signal the sending server that the user is already joined to the
		//    room.
		// For now we will implement option 2. Since in the abesence of a retry
		// mechanism it will be equivalent to option 1, and we don't have a
		// signaling mechanism to implement option 3.
		internal.GetLogger(ctx).Error("user is already joined to room")
		return spec.Forbidden("user is already joined to room")
	}
	return nil
}

func createInviteLogger(ctx context.Context, roomID spec.RoomID, inviter spec.UserID, invitee spec.UserID, eventID string) *logrus.Entry {
	return internal.GetLogger(ctx).WithFields(map[string]any{
		"inviter":  inviter.String(),
		"invitee":  invitee.String(),
		"room_id":  roomID.String(),
		"event_id": eventID,
	})
}

// setUnsignedFieldForInvite stores the stripped state on an invite. It is
// always a list, as clients expect, even when empty.
func setUnsignedFieldForInvite(event PDU, inviteState []InviteStrippedState) error {
	if inviteState == nil {
		inviteState = []InviteStrippedState{}
	}
	if err := event.SetUnsignedField("invite_room_state", inviteState); err != nil {
		return fmt.Errorf("event.SetUnsignedField: %w", err)
	}
	return nil
}

func setUnsignedFieldForProtoInvite(event *ProtoEvent, inviteState []InviteStrippedState) error {
	if inviteState == nil {
		inviteState = []InviteStrippedState{}
	}
	if err := event.SetUnsigned(map[string]any{"invite_room_state": inviteState}); err != nil {
		return fmt.Errorf("event.SetUnsignedField: %w", err)
	}
	return nil
}
