package character

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVisualNameOnlyObservationDoesNotExposeProfileRole(t *testing.T) {
	ctx, facts := visualFixture()
	facts.Exchanges[1].Text = "我是奥罗拉·辛尼斯塔。"
	out, err := Project(ctx, facts)
	if err != nil {
		t.Fatal(err)
	}
	v := out.Knowledge[0].Visual
	if v == nil || v.Role != "" || v.ProfileRevision < 1 {
		t.Fatalf("unlearned role exposed: %+v", v)
	}
	raw, err := json.Marshal(out.Knowledge)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "天文学教授") {
		t.Fatal("knowledge exposes unlearned profile role")
	}
	p := ctx.VisualContext.Subjects["sinistra"].Profile
	if !VisualRoleSnapshotMatches(*v, p) {
		t.Fatal("role snapshot did not bind current role")
	}
	p.Revision++
	if VisualRoleSnapshotMatches(*v, p) {
		t.Fatal("changed profile revision did not invalidate observation")
	}
	if !VisualIdentitySupports(out.Exchanges[1], p, "protagonist") || VisualRecognitionSupports(out.Exchanges[1], p, "protagonist") {
		t.Fatal("name knowledge became role knowledge")
	}
}

func TestVisualRoleSnapshotOldObservationCompatibility(t *testing.T) {
	ctx, facts := visualFixture()
	out, err := Project(ctx, facts)
	if err != nil {
		t.Fatal(err)
	}
	v := out.Knowledge[0].Visual
	if v.Role != "天文学教授" || v.ProfileRevision != 0 || !VisualRoleSnapshotMatches(*v, ctx.VisualContext.Subjects["sinistra"].Profile) {
		t.Fatalf("old role snapshot contract changed: %+v", v)
	}
	v.ProfileRevision = 1
	if VisualRoleSnapshotMatches(*v, ctx.VisualContext.Subjects["sinistra"].Profile) {
		t.Fatal("ambiguous dual snapshot accepted")
	}
}
