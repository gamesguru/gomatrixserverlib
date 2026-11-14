package gomatrixserverlib

import (
	"errors"
	"sync"
)

// ErrUseBuiltin is returned by a ContentHashHook for room versions it does not
// handle, so the built-in check runs instead.
var ErrUseBuiltin = errors.New("gomatrixserverlib: use the built-in implementation")

// EventIDHook computes the reference-hash event ID of eventJSON under
// roomVersion. It must return an error rather than a guess when it cannot.
type EventIDHook func(roomVersion RoomVersion, eventJSON []byte) (string, error)

// ContentHashHook checks the hashes.sha256 of eventJSON under roomVersion. A
// non-nil error means the content hash is invalid, which redacts the event
// exactly as the built-in check does.
type ContentHashHook func(roomVersion RoomVersion, eventJSON []byte) error

var (
	hookMu          sync.RWMutex
	eventIDHook     EventIDHook
	contentHashHook ContentHashHook
)

// SetEventIDHook replaces the built-in reference-hash event ID computation
// for room versions whose IDs are derived from the event (v3 and later).
// Pass nil to restore the built-in one. Call it before parsing events.
func SetEventIDHook(h EventIDHook) {
	hookMu.Lock()
	defer hookMu.Unlock()
	eventIDHook = h
}

// SetContentHashHook replaces the built-in content hash check. Pass nil to
// restore the built-in one. Call it before parsing events.
func SetContentHashHook(h ContentHashHook) {
	hookMu.Lock()
	defer hookMu.Unlock()
	contentHashHook = h
}

func getEventIDHook() EventIDHook {
	hookMu.RLock()
	defer hookMu.RUnlock()
	return eventIDHook
}

func getContentHashHook() ContentHashHook {
	hookMu.RLock()
	defer hookMu.RUnlock()
	return contentHashHook
}

// checkContentHash runs the installed hook, or the built-in check.
func checkContentHash(eventJSON []byte, roomVersion RoomVersion) error {
	if h := getContentHashHook(); h != nil {
		if err := h(roomVersion, eventJSON); !errors.Is(err, ErrUseBuiltin) {
			return err
		}
	}
	return checkEventContentHash(eventJSON)
}
