package httpapi

import "testing"

func TestShouldCompact(t *testing.T) {
	cases := []struct {
		name  string
		count int
		bytes int
		want  bool
	}{
		{"fresh document", 0, 0, false},
		{"ordinary editing session", 40, 60 << 10, false},
		{"long history", compactUpdateCount, 1 << 10, true},
		{"one update short", compactUpdateCount - 1, 1 << 10, false},
		{"few but heavy updates", 12, compactUpdateBytes, true},
		{"heavy but under the limit", 12, compactUpdateBytes - 1, false},
	}
	for _, item := range cases {
		if got := shouldCompact(item.count, item.bytes); got != item.want {
			t.Errorf("%s: shouldCompact(%d, %d) = %v, want %v", item.name, item.count, item.bytes, got, item.want)
		}
	}
}

func TestSnapshotLimitMatchesTheColumnCheck(t *testing.T) {
	// The websocket read limit has to leave room for the channel prefix, and
	// the snapshot cap has to match the CHECK on collab_snapshots.state_data.
	if maxSnapshotBytes != 16<<20 {
		t.Fatalf("maxSnapshotBytes = %d; the migration allows 16 MiB", maxSnapshotBytes)
	}
	if maxUpdateBytes != 1<<20 {
		t.Fatalf("maxUpdateBytes = %d; the migration allows 1 MiB", maxUpdateBytes)
	}
}
