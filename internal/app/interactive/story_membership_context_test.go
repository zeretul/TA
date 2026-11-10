package interactiveapp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"ta/internal/character"
	"ta/internal/interactive"
)

func membershipContextFixture() interactive.WorldState {
	state := renderTestWorld("越国坊市外围散修人家")
	state.FactionMemberships = []character.FactionMembership{{FactionID: "yue.huangfeng", FactionName: "黄枫谷", Status: "pending", Statement: "已当面提出申请，尚未获准"}}
	state.MembershipContacts = []interactive.MembershipAdmissionContact{{CharacterID: "present-admitter", FactionID: "yue.huangfeng", FactionName: "黄枫谷", Statuses: []string{"formal", "temporary"}}}
	return state
}

func TestMembershipContextKeepsKnownStandingAndOtherWorldActions(t *testing.T) {
	state := membershipContextFixture()
	page := renderWorldActions(state, interactive.StoryContextMaxBytes)
	for _, want := range []string{"yue.huangfeng", "status=pending", "已当面提出申请，尚未获准", "申请不保证成功", "不是同意", "present-admitter", "formal/temporary", "route_036:reverse", "quest.herb_handover", "realm.gu_xiu_dong_fu"} {
		if !strings.Contains(page, want) {
			t.Errorf("membership page omitted %q", want)
		}
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "present-admitter") || strings.Contains(string(data), "MembershipContacts") || strings.Contains(string(data), "membership_contacts") {
		t.Fatal("narrator-only admission authority leaked into player JSON")
	}
	if !strings.Contains(string(data), "faction_memberships") || !strings.Contains(string(data), "pending") {
		t.Fatal("known standing missing from player JSON")
	}
}

func TestMembershipContextHonorsStrictByteBudget(t *testing.T) {
	state := membershipContextFixture()
	for i := 0; i < 30; i++ {
		state.FactionMemberships = append(state.FactionMemberships, character.FactionMembership{FactionID: fmt.Sprintf("faction-%d", i), FactionName: "已知组织", Status: "formal", Statement: strings.Repeat("已知的接纳依据", 25)})
	}
	for _, budget := range []int{1, 80, 700, 1500, 2400, 4096, interactive.StoryContextMaxBytes} {
		t.Run(fmt.Sprint(budget), func(t *testing.T) {
			page := renderWorldActions(state, budget)
			if len(page) > budget {
				t.Fatalf("membership context exceeded byte budget: got %d, limit %d", len(page), budget)
			}
			if !utf8.ValidString(page) {
				t.Fatal("budget truncates UTF-8 prose")
			}
		})
	}
}
