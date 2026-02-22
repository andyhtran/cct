package session

import (
	"testing"
	"unicode/utf8"
)

func FuzzFastExtractType(f *testing.F) {
	f.Add([]byte(`{"type":"user","message":{}}`))
	f.Add([]byte(`{"type":"assistant","message":{}}`))
	f.Add([]byte(`{"type":"","message":{}}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`not json at all`))
	f.Add([]byte(`{"message":"has \"type\":\"fake\" inside"}`))
	f.Add([]byte(`{"type":"`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		result := FastExtractType(data)
		if !utf8.ValidString(result) {
			t.Errorf("FastExtractType returned invalid UTF-8: %q", result)
		}
	})
}

func FuzzFastExtractTimestamp(f *testing.F) {
	f.Add([]byte(`{"timestamp":"2026-01-10T08:00:00Z"}`))
	f.Add([]byte(`{"timestamp":"2026-01-10T08:00:00.123456789Z"}`))
	f.Add([]byte(`{"timestamp":"not-a-time"}`))
	f.Add([]byte(`{"timestamp":"`))
	f.Add([]byte(`{}`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		result := FastExtractTimestamp(data)
		// Result is either zero time or a valid time — just verify no panic.
		_ = result.IsZero()
	})
}
