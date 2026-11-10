package interactive

import (
	"testing"

	"ta/internal/cultivation"
)

// The tests here cover the join between the one 天赋特性 a character was created
// with and the attribute aggregation of 《自制-修仙数值规则》§3.4.
//
// The talent's size is the rules' table, and the character's state carries only
// its name. That is what makes the bonus impossible to duplicate: a retry, a
// re-read or a fork that copies the state still yields exactly one modifier.

func TestAnInnateTalentRaisesExactlyOneSettledAttribute(t *testing.T) {
	// At qi-refining layer 6 the bare table is 200 health, 160 mana, 40 attack
	// and 20 for defence, agility and perception. 体魄强健 is +5% health max.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldTalent: "体魄强健",
	})
	settled := xiuxianSettle(t, state)

	if got := xiuxianSettledNumber(t, settled, xiuxianFieldHealthMax); got != 210 {
		t.Errorf("health max with 体魄强健 = %v, want 210 (200 × 1.05)", got)
	}
	// A talent names one attribute; the other five must stay at the bare table.
	for _, field := range []string{xiuxianFieldManaMax, xiuxianFieldAttack, xiuxianFieldDefense, xiuxianFieldAgility, xiuxianFieldPerception} {
		if got := xiuxianSettledNumber(t, settled, field); got == 0 {
			t.Errorf("%s was not settled", field)
		}
	}
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 40 {
		t.Errorf("attack with a health talent = %v, want the bare 40", got)
	}
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldPerception); got != 20 {
		t.Errorf("perception with a health talent = %v, want the bare 20", got)
	}
}

func TestEveryTalentMovesItsOwnAttributeByFivePercent(t *testing.T) {
	// The published table pairs each talent with exactly one of the six
	// attributes. This asserts the pairing the player is shown is the one the
	// settlement pays, for all six rather than the one spot-checked above.
	want := map[string]string{
		"体魄强健": xiuxianFieldHealthMax,
		"气脉绵长": xiuxianFieldManaMax,
		"眼明心细": xiuxianFieldPerception,
		"身轻步快": xiuxianFieldAgility,
		"出手凌厉": xiuxianFieldAttack,
		"筋骨坚实": xiuxianFieldDefense,
	}
	// The bare values at this stage, so the test can name the expected result
	// without recomputing the rules' own arithmetic.
	bare := map[string]float64{
		xiuxianFieldHealthMax:  200,
		xiuxianFieldManaMax:    160,
		xiuxianFieldAttack:     40,
		xiuxianFieldDefense:    20,
		xiuxianFieldAgility:    20,
		xiuxianFieldPerception: 20,
	}
	for name, attribute := range want {
		state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
			xiuxianFieldTalent: name,
		})
		settled := xiuxianSettle(t, state)
		expected := bare[attribute] * 1.05
		if got := xiuxianSettledNumber(t, settled, attribute); got != expected {
			t.Errorf("%s raised %s to %v, want %v", name, attribute, got, expected)
		}
		for other, otherBare := range bare {
			if other == attribute {
				continue
			}
			if got := xiuxianSettledNumber(t, settled, other); got != otherBare {
				t.Errorf("%s changed %s to %v, want the bare %v", name, other, got, otherBare)
			}
		}
	}
}

func TestATalentIsNotCountedTwiceWhenTheStateIsReadAgain(t *testing.T) {
	// A settlement is applied, then settled again from its own output — which is
	// what a retried commit and a resumed branch both do. The bonus must land
	// once: the state carries a name, so the second pass reads the same name and
	// produces the same single modifier rather than adding a second one.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldTalent: "出手凌厉",
	})
	first := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, first, xiuxianFieldAttack); got != 42 {
		t.Fatalf("attack after the first settlement = %v, want 42 (40 × 1.05)", got)
	}

	second := xiuxianSettle(t, first)
	if got := xiuxianSettledNumber(t, second, xiuxianFieldAttack); got != 42 {
		t.Errorf("attack after settling the settled state = %v, want 42; the talent was counted twice", got)
	}
}

func TestATalentTheRulesDoNotKnowContributesNothing(t *testing.T) {
	// A hand-edited save, or a talent from a pack whose rules are newer than this
	// build, must not be priced by guesswork. The character keeps the bare table.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldTalent: "天生神力",
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 40 {
		t.Errorf("attack with an unknown talent = %v, want the bare 40", got)
	}
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldHealthMax); got != 200 {
		t.Errorf("health max with an unknown talent = %v, want the bare 200", got)
	}
}

func TestACharacterWithNoTalentIsUnchanged(t *testing.T) {
	// An existing save has no 天赋特性 field at all. The settlement must produce
	// the bare table, which is what makes 旧存档不回填天赋 hold without a migration.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)
	settled := xiuxianSettle(t, state)
	for field, want := range map[string]float64{
		xiuxianFieldHealthMax:  200,
		xiuxianFieldManaMax:    160,
		xiuxianFieldAttack:     40,
		xiuxianFieldDefense:    20,
		xiuxianFieldAgility:    20,
		xiuxianFieldPerception: 20,
	} {
		if got := xiuxianSettledNumber(t, settled, field); got != want {
			t.Errorf("%s without a talent = %v, want the bare %v", field, got, want)
		}
	}
}

func TestAFreshCharacterStartsAtTheRaisedPools(t *testing.T) {
	// 健康新角色初始化 HP/MP 匹配新上限. The state has no recorded wound, so the
	// settlement seeds the current pool from the derived maximum — which now
	// includes the talent. A wound already recorded is the opposite case and is
	// covered by TestAnInnateTalentDoesNotHealAnExistingWound.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldTalent: "体魄强健",
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldHealth); got != 210 {
		t.Errorf("a fresh character's health = %v, want the raised maximum 210", got)
	}
}

func TestAnInnateTalentDoesNotHealAnExistingWound(t *testing.T) {
	// 既有角色不得因加成自动回血. A character already at 50 of 200 gains the
	// talent's headroom, not a heal: health stays 50 and only the ceiling moves.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldHealth: float64(50),
		xiuxianFieldMana:   float64(80),
		xiuxianFieldTalent: "体魄强健",
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldHealth); got != 50 {
		t.Errorf("a wounded character's health = %v, want 50 (a talent must not heal)", got)
	}
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldHealthMax); got != 210 {
		t.Errorf("health max = %v, want 210", got)
	}
}

func TestTheTalentStaysInsideThePublishedCap(t *testing.T) {
	// 复用既有加成上限. The talent is an ordinary positive modifier, so it shares
	// the 40% pool ceiling with everything else rather than sitting outside it.
	// A single 5% talent cannot reach the cap on its own, which is the point:
	// the cap is enforced by the aggregation, not by this table.
	modifiers := []cultivation.AttributeModifier{}
	for _, talent := range cultivation.InnateTalents() {
		modifiers = append(modifiers, cultivation.InnateTalentModifier(talent))
	}
	// Six talents on six different attributes cannot be held at once — a
	// character has at most one — but the aggregation must still cap each
	// attribute independently rather than summing them into one.
	breakdown := cultivation.AggregateAttribute(cultivation.AttrHealthMax, 200, modifiers)
	if breakdown.Value != 210 {
		t.Errorf("health max with the whole table = %v, want 210; only 体魄强健 names this attribute", breakdown.Value)
	}
}
