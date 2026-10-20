package gomatrixserverlib

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"github.com/gamesguru/gomatrixserverlib/spec"
)

// referenceKahnsUsingPrevEvents is the original implementation of
// kahnsAlgorithmUsingPrevEvents, which re-sorted the whole queue after every
// pop. It is kept here to prove the heap-based version orders identically.
func referenceKahnsUsingPrevEvents(events []*stateResV2ConflictedOther) []*stateResV2ConflictedOther {
	eventMap := make(map[string]*stateResV2ConflictedOther, len(events))
	graph := make([]*stateResV2ConflictedOther, 0, len(events))
	inDegree := make(map[string]int, len(events))
	for _, event := range events {
		eventMap[event.eventID] = event
		if _, ok := inDegree[event.eventID]; !ok {
			inDegree[event.eventID] = 0
		}
		for _, prev := range event.event.PrevEventIDs() {
			inDegree[prev]++
		}
	}
	noIncoming := make(stateResV2ConflictedOtherHeap, 0, len(events))
	for eventID, count := range inDegree {
		if count == 0 {
			noIncoming = append(noIncoming, eventMap[eventID])
			delete(eventMap, eventID)
		}
	}
	slices.SortStableFunc(noIncoming, sortStateResV2ConflictedOtherHeap)
	for len(noIncoming) > 0 {
		event := noIncoming[len(noIncoming)-1]
		noIncoming = noIncoming[:len(noIncoming)-1]
		graph = append([]*stateResV2ConflictedOther{event}, graph...)
		for _, prev := range event.event.PrevEventIDs() {
			inDegree[prev]--
			if inDegree[prev] == 0 {
				if _, ok := eventMap[prev]; ok {
					noIncoming = append(noIncoming, eventMap[prev])
					delete(eventMap, prev)
				}
			}
		}
		slices.SortStableFunc(noIncoming, sortStateResV2ConflictedOtherHeap)
	}
	if len(eventMap) > 0 {
		remaining := make(stateResV2ConflictedOtherHeap, 0, len(events))
		for _, event := range eventMap {
			remaining = append(remaining, event)
		}
		slices.SortStableFunc(remaining, sortStateResV2ConflictedOtherHeap)
		graph = append(remaining, graph...)
	}
	return graph
}

// referenceKahnsUsingAuthEvents is the original implementation of
// kahnsAlgorithmUsingAuthEvents, kept for the same reason.
func referenceKahnsUsingAuthEvents(events []*stateResV2ConflictedPowerLevel) []*stateResV2ConflictedPowerLevel {
	eventMap := make(map[string]*stateResV2ConflictedPowerLevel, len(events))
	graph := make([]*stateResV2ConflictedPowerLevel, 0, len(events))
	inDegree := make(map[string]int, len(events))
	for _, event := range events {
		eventMap[event.eventID] = event
		if _, ok := inDegree[event.eventID]; !ok {
			inDegree[event.eventID] = 0
		}
		for _, auth := range event.event.AuthEventIDs() {
			inDegree[auth]++
		}
	}
	noIncoming := make(stateResV2ConflictedPowerLevelHeap, 0, len(events))
	for eventID, count := range inDegree {
		if count == 0 {
			noIncoming = append(noIncoming, eventMap[eventID])
			delete(eventMap, eventID)
		}
	}
	slices.SortStableFunc(noIncoming, sortStateResV2ConflictedPowerLevelHeap)
	for len(noIncoming) > 0 {
		event := noIncoming[len(noIncoming)-1]
		noIncoming = noIncoming[:len(noIncoming)-1]
		graph = append([]*stateResV2ConflictedPowerLevel{event}, graph...)
		for _, auth := range event.event.AuthEventIDs() {
			inDegree[auth]--
			if inDegree[auth] == 0 {
				if _, ok := eventMap[auth]; ok {
					noIncoming = append(noIncoming, eventMap[auth])
					delete(eventMap, auth)
				}
			}
		}
		slices.SortStableFunc(noIncoming, sortStateResV2ConflictedPowerLevelHeap)
	}
	if len(eventMap) > 0 {
		remaining := make(stateResV2ConflictedPowerLevelHeap, 0, len(events))
		for _, event := range eventMap {
			remaining = append(remaining, event)
		}
		slices.SortStableFunc(remaining, sortStateResV2ConflictedPowerLevelHeap)
		graph = append(remaining, graph...)
	}
	return graph
}

// randomKahnGraph builds n events whose prev and auth events point at random
// earlier events, at IDs outside the set, and occasionally at later events so
// that cycles leave stray events behind. Timestamps and power levels are drawn
// from small ranges so the sort has to fall through to its tiebreaks.
func randomKahnGraph(rng *rand.Rand, n int) []*eventV1 {
	id := func(i int) string { return fmt.Sprintf("$%05d:example.com", i) }
	events := make([]*eventV1, n)
	for i := range events {
		refs := func() []eventReference {
			var out []eventReference
			for range rng.Intn(4) {
				switch r := rng.Intn(10); {
				case r == 0:
					out = append(out, eventReference{EventID: fmt.Sprintf("$missing%d:example.com", rng.Intn(n))})
				case r == 1:
					out = append(out, eventReference{EventID: id(rng.Intn(n))})
				case i > 0:
					out = append(out, eventReference{EventID: id(rng.Intn(i))})
				}
			}
			return out
		}
		events[i] = &eventV1{
			roomVersion: RoomVersionV2,
			EventIDRaw:  id(i),
			eventFields: eventFields{
				RoomID:         "!ROOM:example.com",
				Type:           spec.MRoomMember,
				OriginServerTS: spec.Timestamp(rng.Intn(5)),
			},
			PrevEvents: refs(),
			AuthEvents: refs(),
		}
	}
	return events
}

func TestKahnsAlgorithmMatchesReference(t *testing.T) {
	for seed := int64(0); seed < 200; seed++ {
		rng := rand.New(rand.NewSource(seed))
		events := randomKahnGraph(rng, 1+rng.Intn(300))

		others := make([]*stateResV2ConflictedOther, len(events))
		powerLevels := make([]*stateResV2ConflictedPowerLevel, len(events))
		for i, ev := range events {
			others[i] = &stateResV2ConflictedOther{
				mainlinePosition: rng.Intn(3),
				mainlineSteps:    rng.Intn(3),
				originServerTS:   ev.OriginServerTS(),
				eventID:          ev.EventID(),
				event:            ev,
			}
			powerLevels[i] = &stateResV2ConflictedPowerLevel{
				powerLevel:     int64(rng.Intn(3) * 50),
				originServerTS: ev.OriginServerTS(),
				eventID:        ev.EventID(),
				event:          ev,
			}
		}

		gotPrev := kahnsAlgorithmUsingPrevEvents(slices.Clone(others))
		wantPrev := referenceKahnsUsingPrevEvents(slices.Clone(others))
		if !slices.Equal(gotPrev, wantPrev) {
			t.Fatalf("seed %d: prev event ordering differs from reference", seed)
		}

		gotAuth := kahnsAlgorithmUsingAuthEvents(slices.Clone(powerLevels))
		wantAuth := referenceKahnsUsingAuthEvents(slices.Clone(powerLevels))
		if !slices.Equal(gotAuth, wantAuth) {
			t.Fatalf("seed %d: auth event ordering differs from reference", seed)
		}
	}
}

// BenchmarkKahnsAlgorithmUsingPrevEvents orders the full state of a large
// room, where no state event references another as a prev event.
func BenchmarkKahnsAlgorithmUsingPrevEvents(b *testing.B) {
	const n = 50000
	others := make([]*stateResV2ConflictedOther, n)
	for i := range others {
		ev := &eventV1{
			roomVersion: RoomVersionV2,
			EventIDRaw:  fmt.Sprintf("$%05d:example.com", i),
			eventFields: eventFields{OriginServerTS: spec.Timestamp(i)},
			PrevEvents:  []eventReference{{EventID: fmt.Sprintf("$prev%05d:example.com", i)}},
		}
		others[i] = &stateResV2ConflictedOther{
			originServerTS: ev.OriginServerTS(),
			eventID:        ev.EventID(),
			event:          ev,
		}
	}
	for b.Loop() {
		kahnsAlgorithmUsingPrevEvents(others)
	}
}
