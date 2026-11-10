package character

import (
	"strings"
	"testing"
)

func TestAuthoredPartnerProtectionFollowsIdentityNotDisplayName(t *testing.T) {
	for _, key := range []string{"npc_nan_gongwan", "npc_han_li", "xinghai-p01", "xinghai-p02"} {
		got := Prepare(PrepareInput{Profile: Profile{ID: "renamed", Name: "别名", SourceKey: key}}).Markdown()
		if !strings.Contains(got, "作者关系边界") {
			t.Fatalf("missing protected identity %s", key)
		}
	}
	unrelated := Prepare(PrepareInput{Profile: Profile{ID: "new", Name: "南宫婉"}}).Markdown()
	if strings.Contains(unrelated, "作者关系边界：你是南宫婉") {
		t.Fatal("name alone captured a new person")
	}
	got := Prepare(PrepareInput{Profile: Profile{ID: "mu", Name: "慕沛灵", SourceKey: "npc_mu_peiling"}}).Markdown()
	if !strings.Contains(got, "非正妻") || !strings.Contains(got, "不能默认答应") {
		t.Fatal("non-primary consent boundary missing")
	}
}
