package gomatrixserverlib

import (
	"context"
	"errors"
	"fmt"

	"github.com/tidwall/gjson"

	"github.com/gamesguru/gomatrixserverlib/spec"
)

// ErrStrippedStateMissingCreate is returned by ValidateStrippedStatePDUs
// when no valid m.room.create event is among the entries.
var ErrStrippedStateMissingCreate = errors.New("m.room.create event not among room state entries")

// ErrStrippedStateNotPDU is wrapped by the entry errors of
// ValidateStrippedStatePDUs for entries which are not PDUs of the room
// version, such as the stripped state sent by servers predating MSC4311.
var ErrStrippedStateNotPDU = errors.New("not a PDU for the room version")

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

// StrippedStateRejectsInvite reports whether an invite must be rejected
// for invite_room_state which failed ValidateStrippedStatePDUs with err.
//
// MSC4311 makes rejection a MAY for room versions 1 through 11, so those
// only warn. For later room versions its migration section asks servers to
// warn rather than fail while servers catch up with sending full PDUs, but
// to still fail when the state holds PDUs for a different room. So an
// invite is rejected when some entry is a PDU which fails validation, and
// accepted when entries merely are not PDUs or the create event is missing.
func StrippedStateRejectsInvite(roomVersion RoomVersion, err error) bool {
	if err == nil {
		return false
	}
	verImpl, verErr := GetRoomVersion(roomVersion)
	if verErr != nil || !verImpl.DomainlessRoomIDs() {
		return false
	}
	problems := []error{err}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		problems = joined.Unwrap()
	}
	for _, problem := range problems {
		if errors.Is(problem, ErrStrippedStateMissingCreate) || errors.Is(problem, ErrStrippedStateNotPDU) {
			continue
		}
		return true
	}
	return false
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
		return nil, ErrStrippedStateNotPDU
	}
	event, err := verImpl.NewEventFromUntrustedJSON(eventJSON)
	if err != nil {
		return nil, fmt.Errorf("%w %s: %w", ErrStrippedStateNotPDU, verImpl.Version(), err)
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
