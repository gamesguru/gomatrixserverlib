package gomatrixserverlib

import (
	"context"
	"errors"
	"fmt"

	"github.com/tidwall/gjson"

	"codefloe.com/pat-s/gomatrixserverlib/spec"
)

// ErrStrippedStateMissingCreate is returned by ValidateStrippedStatePDUs
// when no valid m.room.create event is among the entries.
var ErrStrippedStateMissingCreate = errors.New("m.room.create event not among room state entries")

// StrippedStateEntryError describes why a single invite_room_state or
// knock_room_state entry failed validation.
type StrippedStateEntryError struct {
	Index int    // The position of the entry in the received list
	Type  string // The claimed event type, if any
	Err   error
}

func (e StrippedStateEntryError) Error() string {
	return fmt.Sprintf("room state entry %d (%q): %s", e.Index, e.Type, e.Err)
}

func (e StrippedStateEntryError) Unwrap() error {
	return e.Err
}

// EnforceStrippedStateValidation reports whether invites to rooms of the
// given version must be rejected when their invite_room_state fails
// ValidateStrippedStatePDUs. MSC4311 makes rejection a SHOULD for room
// versions with room IDs derived from the create event (v12 and later) and
// a MAY for older versions, where servers are expected to warn instead
// while the federation migrates.
func EnforceStrippedStateValidation(roomVersion RoomVersion) bool {
	verImpl, err := GetRoomVersion(roomVersion)
	if err != nil {
		return false
	}
	return verImpl.DomainlessRoomIDs()
}

// ValidateStrippedStatePDUs validates invite_room_state or knock_room_state
// received over federation as required by MSC4311 (Matrix 1.16). Every
// entry must be a PDU formatted for the room version, belong to roomID and
// pass signature checks, and a valid m.room.create event must be present.
// In room versions with domainless room IDs the room ID is derived from the
// create event's reference hash rather than read from the event.
//
// It returns the entries which passed, rebuilt from the parsed events so
// that their stripped form reflects the verified event, and an error
// joining every problem found. Callers decide whether a non-nil error
// rejects the request (invites) or whether only the valid entries are kept
// (knocks). Verification needs the signing keys of every event's origin, so
// this can make outbound key requests.
func ValidateStrippedStatePDUs(
	ctx context.Context,
	roomID spec.RoomID,
	roomVersion RoomVersion,
	state []InviteStrippedState,
	verifier JSONVerifier,
	userIDForSender spec.UserIDForSender,
) ([]InviteStrippedState, error) {
	verImpl, err := GetRoomVersion(roomVersion)
	if err != nil {
		return nil, err
	}

	var problems []error
	valid := make([]InviteStrippedState, 0, len(state))
	haveCreate := false
	for idx := range state {
		entry := &state[idx]
		event, err := validateStrippedStatePDU(ctx, verImpl, roomID, entry.PDUJSON(), verifier, userIDForSender)
		if err != nil {
			problems = append(problems, StrippedStateEntryError{Index: idx, Type: entry.Type(), Err: err})
			continue
		}
		if event.Type() == spec.MRoomCreate && event.StateKeyEquals("") {
			haveCreate = true
		}
		valid = append(valid, NewInviteStrippedState(event))
	}
	if !haveCreate {
		problems = append(problems, ErrStrippedStateMissingCreate)
	}
	return valid, errors.Join(problems...)
}

func validateStrippedStatePDU(
	ctx context.Context,
	verImpl IRoomVersion,
	roomID spec.RoomID,
	eventJSON spec.RawJSON,
	verifier JSONVerifier,
	userIDForSender spec.UserIDForSender,
) (PDU, error) {
	if len(eventJSON) == 0 {
		return nil, errors.New("not a PDU")
	}
	event, err := verImpl.NewEventFromUntrustedJSON(eventJSON)
	if err != nil {
		return nil, fmt.Errorf("not a PDU for room version %s: %w", verImpl.Version(), err)
	}
	if event.StateKey() == nil {
		return nil, errors.New("not a state event")
	}

	isCreate := event.Type() == spec.MRoomCreate && event.StateKeyEquals("")
	var eventRoomID string
	if isCreate && verImpl.DomainlessRoomIDs() {
		// The room ID is the create event's reference hash with a '!' sigil.
		eventRoomID = event.RoomID().String()
	} else {
		// Read the field directly: PDU.RoomID panics on malformed room IDs.
		eventRoomID = gjson.GetBytes(eventJSON, "room_id").Str
	}
	if eventRoomID != roomID.String() {
		return nil, fmt.Errorf("event belongs to room %q, not %q", eventRoomID, roomID.String())
	}

	if isCreate {
		createRoomVersion := RoomVersionV1
		if v := gjson.GetBytes(event.Content(), "room_version"); v.Exists() {
			createRoomVersion = RoomVersion(v.String())
		}
		if createRoomVersion != verImpl.Version() {
			return nil, fmt.Errorf("create event is for room version %q, not %q", createRoomVersion, verImpl.Version())
		}
	}

	if err := VerifyEventSignatures(ctx, event, verifier, userIDForSender); err != nil {
		return nil, fmt.Errorf("signature check failed: %w", err)
	}
	return event, nil
}
