package interactive

import "testing"

// A new revision or an unfamiliar pack name must not require a code whitelist.
func TestShippedWorldGameplayUsesFrozenOwnershipWithoutRevisionWhitelist(t *testing.T) {
	for _, id := range []string{"fanren-jie", "lanchuan-qunxia", "xinghai-689", "sui-guan-realms"} {
		c := authoredWorldContent(t, id)
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
		meta := StoryMeta{PackBinding: &StoryPackBinding{PackID: id, ContentVersion: 9999}, ActorStateSchema: &ActorStateSchemaSnapshot{System: StoryDirectorActorStateSystem{WorldContent: c}}}
		if !storyPackAllows(meta, id, "1") || storyPackAllows(meta, "foreign-world", "1") {
			t.Fatal("world ownership depends on revision whitelist", id)
		}
		c = CloneWorldContent(c)
		c.PackID = "custom-unfamiliar"
		meta.PackBinding.PackID = c.PackID
		meta.ActorStateSchema.System.WorldContent = c
		if !storyPackAllows(meta, c.PackID, "1") {
			t.Fatal("unfamiliar world refused")
		}
	}
}
