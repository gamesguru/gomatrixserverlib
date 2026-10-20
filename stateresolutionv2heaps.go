// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"strings"

	"codefloe.com/pat-s/gomatrixserverlib/spec"
)

// A stateResV2ConflictedPowerLevel is used to sort the events by effective
// power level, origin server TS and the lexicographical comparison of event
// IDs. It is a bit of an optimisation to use this - by working out the
// effective power level etc ahead of time, we use less CPU cycles during the
// sort.
type stateResV2ConflictedPowerLevel struct {
	powerLevel     int64
	originServerTS spec.Timestamp
	eventID        string
	event          PDU
}

// A stateResV2ConflictedPowerLevelHeap is used to sort the events using
// sort.Sort or by using the heap functions for further optimisation. Sorting
// ensures that the results are deterministic.
type stateResV2ConflictedPowerLevelHeap []*stateResV2ConflictedPowerLevel

// Less implements sort.Interface.
func sortStateResV2ConflictedPowerLevelHeap(a, b *stateResV2ConflictedPowerLevel) int {
	// Try to tiebreak on the effective power level
	if a.powerLevel > b.powerLevel {
		return -1
	}
	if a.powerLevel < b.powerLevel {
		return 1
	}
	// If we've reached here then s[i].powerLevel == s[j].powerLevel
	// so instead try to tiebreak on origin server TS
	if a.originServerTS < b.originServerTS {
		return -1
	}
	if a.originServerTS > b.originServerTS {
		return 1
	}
	// If we've reached here then s[i].originServerTS == s[j].originServerTS
	// so instead try to tiebreak on a lexicographical comparison of the event ID
	return strings.Compare(a.eventID, b.eventID)
}

// A stateResV2ConflictedOther is used to sort the events by power level
// mainline positions, origin server TS and the lexicographical comparison of
// event IDs. It is a bit of an optimisation to use this - by working out the
// effective power level etc ahead of time, we use less CPU cycles during the
// sort.
type stateResV2ConflictedOther struct {
	mainlinePosition int
	mainlineSteps    int
	originServerTS   spec.Timestamp
	eventID          string
	event            PDU
}

// A stateResV2ConflictedOtherHeap is used to sort the events using
// sort.Sort or by using the heap functions for further optimisation. Sorting
// ensures that the results are deterministic.
type stateResV2ConflictedOtherHeap []*stateResV2ConflictedOther

func sortStateResV2ConflictedOtherHeap(a, b *stateResV2ConflictedOther) int {
	// Try to tiebreak on the mainline position
	if a.mainlinePosition < b.mainlinePosition {
		return -1
	}
	if a.mainlinePosition > b.mainlinePosition {
		return 1
	}
	// If we've reached here then s[i].mainlinePosition == s[j].mainlinePosition
	// so instead try to tiebreak on step count
	if a.mainlineSteps < b.mainlineSteps {
		return -1
	}
	if a.mainlineSteps > b.mainlineSteps {
		return 1
	}
	// If we've reached here then s[i].mainlineSteps == s[j].mainlineSteps
	// so instead try to tiebreak on origin server TS
	if a.originServerTS < b.originServerTS {
		return -1
	}
	if a.originServerTS > b.originServerTS {
		return 1
	}
	// If we've reached here then s[i].originServerTS == s[j].originServerTS
	// so instead try to tiebreak on a lexicographical comparison of the event ID
	return strings.Compare(a.eventID, b.eventID)
}

// kahnHeap is a binary max-heap of the events that are ready to be placed
// into the graph during Kahn's algorithm. The pop method returns the event
// that sorts last according to cmp, which is the event the algorithm places
// next.
// Keeping a heap rather than re-sorting the whole queue after every pop makes
// ordering n events O(n log n) instead of O(n² log n).
type kahnHeap[T any] struct {
	items []T
	cmp   func(a, b T) int
}

// kahnHeapArity is the number of children of each node in a kahnHeap.
const kahnHeapArity = 2

func newKahnHeap[T any](capacity int, cmp func(a, b T) int) *kahnHeap[T] {
	return &kahnHeap[T]{items: make([]T, 0, capacity), cmp: cmp}
}

func (h *kahnHeap[T]) len() int {
	return len(h.items)
}

// above reports whether the item at i belongs above the item at j.
func (h *kahnHeap[T]) above(i, j int) bool {
	return h.cmp(h.items[i], h.items[j]) > 0
}

func (h *kahnHeap[T]) push(x T) {
	h.items = append(h.items, x)
	for i := len(h.items) - 1; i > 0; {
		parent := (i - 1) / kahnHeapArity
		if !h.above(i, parent) {
			break
		}
		h.items[i], h.items[parent] = h.items[parent], h.items[i]
		i = parent
	}
}

func (h *kahnHeap[T]) pop() T {
	top := h.items[0]
	last := len(h.items) - 1
	h.items[0] = h.items[last]
	var zero T
	h.items[last] = zero
	h.items = h.items[:last]
	for i := 0; ; {
		largest := i
		left := kahnHeapArity*i + 1
		if left < last && h.above(left, largest) {
			largest = left
		}
		if right := left + 1; right < last && h.above(right, largest) {
			largest = right
		}
		if largest == i {
			break
		}
		h.items[i], h.items[largest] = h.items[largest], h.items[i]
		i = largest
	}
	return top
}
