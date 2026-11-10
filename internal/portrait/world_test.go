package portrait

import (
	"strings"
	"ta/internal/catalogart"
	"testing"
)

func TestWorldPortraitsKeepStableIdentityAndLegacyFallback(t *testing.T) {
	l := NewLibrary()
	for _, display := range []bool{false, true} {
		if got := l.WorldURL("lanchuan-qunxia", "lanchuan-p01", display); !strings.HasPrefix(got, catalogart.PathPrefix) {
			t.Fatalf("pilot missing: %q", got)
		}
		for _, world := range []string{"", "fanren-jie", "xinghai-689"} {
			if got := l.WorldURL(world, "lanchuan-p01", display); got != "" {
				t.Fatalf("pilot crossed world %q: %s", world, got)
			}
		}
	}
	key := l.Keys()[0]
	if got := l.WorldURL("fanren-jie", key, false); got != l.URL(key) {
		t.Fatal("legacy portrait changed")
	}
	if got := l.WorldURL("lanchuan-qunxia", key, false); got != "" {
		t.Fatal("legacy portrait crossed world")
	}
}
