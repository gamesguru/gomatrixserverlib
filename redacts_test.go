package gomatrixserverlib

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactsMSC2174(t *testing.T) {
	const target = "$target:localhost"
	roomID := map[RoomVersion]string{
		RoomVersionV10:       "!room:localhost",
		RoomVersionV11:       "!room:localhost",
		RoomVersionV12:       "!L6nFTAu28CEi9yn9up1SUiKtTNnKt2yomgy2JFRT2Zk",
		RoomVersionHydra:     "!L6nFTAu28CEi9yn9up1SUiKtTNnKt2yomgy2JFRT2Zk",
		RoomVersionStateDAGs: "!L6nFTAu28CEi9yn9up1SUiKtTNnKt2yomgy2JFRT2Zk",
	}
	inContent := map[RoomVersion]bool{
		RoomVersionV10:       false,
		RoomVersionV11:       true,
		RoomVersionV12:       true,
		RoomVersionHydra:     true,
		RoomVersionStateDAGs: true,
	}

	tests := []struct {
		name      string
		eventType string
		topLevel  any
		content   map[string]any
		// want is indexed by whether the room version reads content.redacts.
		want map[bool]string
	}{
		{
			name:      "top-level only",
			eventType: "m.room.redaction",
			topLevel:  target,
			content:   map[string]any{},
			want:      map[bool]string{false: target, true: ""},
		},
		{
			name:      "content only",
			eventType: "m.room.redaction",
			content:   map[string]any{"redacts": target},
			want:      map[bool]string{false: "", true: target},
		},
		{
			name:      "both locations",
			eventType: "m.room.redaction",
			topLevel:  "$other:localhost",
			content:   map[string]any{"redacts": target},
			want:      map[bool]string{false: "$other:localhost", true: target},
		},
		{
			name:      "non-string content.redacts",
			eventType: "m.room.redaction",
			content:   map[string]any{"redacts": 42},
			want:      map[bool]string{false: "", true: ""},
		},
		{
			name:      "content.redacts on another event type",
			eventType: "m.room.message",
			content:   map[string]any{"redacts": target, "body": "hi"},
			want:      map[bool]string{false: "", true: ""},
		},
	}

	for ver, rid := range roomID {
		verImpl := MustGetRoomVersion(ver)
		assert.Equal(t, inContent[ver], verImpl.RedactsInContent(), "room version %s", ver)
		for _, tc := range tests {
			t.Run(string(ver)+"/"+tc.name, func(t *testing.T) {
				m := map[string]any{
					"room_id":          rid,
					"type":             tc.eventType,
					"sender":           "@user:localhost",
					"content":          tc.content,
					"origin_server_ts": 1,
					"depth":            1,
					"prev_events":      []string{},
					"auth_events":      []string{},
				}
				if tc.topLevel != nil {
					m["redacts"] = tc.topLevel
				}
				b, err := json.Marshal(m)
				require.NoError(t, err)
				ev, err := verImpl.NewEventFromTrustedJSONWithEventID("$ev:localhost", b, false)
				require.NoError(t, err)
				assert.Equal(t, tc.want[inContent[ver]], ev.Redacts())
			})
		}
	}
}
