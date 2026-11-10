package interactive

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	interactivestate "ta/internal/interactive/state"
)

const playerAttentionRoot = "player_attention"
const PlayerAttentionLimit = 32

var (
	ErrAttentionUnknown  = errors.New("attention target is not known to the player")
	ErrAttentionLimit    = errors.New("attention limit reached")
	ErrAttentionConflict = errors.New("attention state changed; reload before saving")
	ErrAttentionInvalid  = errors.New("invalid attention request")
)

// PlayerAttention is a player-authored reading preference, not an Actor fact.
// Only the player API writes this versioned root through the canonical journal.
type PlayerAttention struct {
	V          int      `json:"v"`
	Characters []string `json:"characters"`
	Quests     []string `json:"quests"`
}

type PlayerAttentionView struct {
	PlayerAttention
	Head string `json:"head"`
}

// AttentionUpdate sets one preference, rather than toggling it, so retries are
// idempotent. ExpectedHead protects unrelated writes and competing edits.
type AttentionUpdate struct {
	BranchID     string `json:"branch"`
	ExpectedHead string `json:"expected_head"`
	Kind         string `json:"kind"`
	ID           string `json:"id"`
	Following    bool   `json:"following"`
}

func PlayerAttentionFromState(state map[string]any) PlayerAttention {
	value := PlayerAttention{V: 1, Characters: []string{}, Quests: []string{}}
	if raw, ok := state[playerAttentionRoot]; ok {
		data, err := json.Marshal(raw)
		if err != nil || json.Unmarshal(data, &value) != nil || value.V != 1 {
			return PlayerAttention{V: 1, Characters: []string{}, Quests: []string{}}
		}
	}
	return value
}

// PlayerAttentionContext is bounded by 64 IDs of at most 160 bytes each plus
// fixed prose (<12 KiB). It contains no profiles, secrets or inferred news.
func PlayerAttentionContext(state map[string]any) string {
	a := PlayerAttentionFromState(state)
	if len(a.Characters)+len(a.Quests) == 0 {
		return ""
	}
	return fmt.Sprintf("\n\n## 玩家关注（来源：本分支正式关注记录）\n人物ID：%s\n事项ID：%s\n这些只是已知对象的检索与排序偏好。优先回读相关已知事实；关注不表示在场、接取、同意或新的消息，不改变期限，不创造事件，不揭露秘密。\n", strings.Join(a.Characters, "、"), strings.Join(a.Quests, "、"))
}

func (s *Store) PlayerAttention(storyID, branchID string) (PlayerAttentionView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return PlayerAttentionView{}, err
	}
	defer release()
	meta, _, err := s.readStoryLocked(storyID)
	if err != nil {
		return PlayerAttentionView{}, err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	p, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return PlayerAttentionView{}, err
	}
	return PlayerAttentionView{PlayerAttention: PlayerAttentionFromState(p.State), Head: p.Head}, nil
}

func (s *Store) UpdatePlayerAttention(storyID string, in AttentionUpdate) (PlayerAttentionView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if in.ID == "" || strings.TrimSpace(in.ID) != in.ID || len(in.ID) > 160 || (in.Kind != "character" && in.Kind != "quest") {
		return PlayerAttentionView{}, ErrAttentionInvalid
	}
	release, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return PlayerAttentionView{}, err
	}
	defer release()
	meta, _, err := s.readStoryLocked(storyID)
	if err != nil {
		return PlayerAttentionView{}, err
	}
	if in.BranchID == "" {
		in.BranchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[in.BranchID]
	if !ok {
		return PlayerAttentionView{}, ErrAttentionInvalid
	}
	p, err := s.storyBranchProjectionLocked(storyID, in.BranchID)
	if err != nil {
		return PlayerAttentionView{}, err
	}
	attention := PlayerAttentionFromState(p.State)
	ids := &attention.Quests
	if in.Kind == "character" {
		ids = &attention.Characters
	}
	index := slices.Index(*ids, in.ID)
	// A repeated desired state has no side effects, even after a lost response.
	if (index >= 0) == in.Following {
		return PlayerAttentionView{attention, p.Head}, nil
	}
	if branch.Head != in.ExpectedHead {
		return PlayerAttentionView{}, ErrAttentionConflict
	}
	if in.Following {
		known := false
		switch in.Kind {
		case "character":
			if p.Characters != nil {
				_, known = p.Characters.Profile(in.ID)
			}
		case "quest":
			_, known = p.Quests[in.ID]
			if !known {
				if def, ok := worldQuestDefinition(worldContentForMeta(meta), in.ID); ok {
					known = questDefinitionEligible(meta, def) && questPubliclyKnown(p, def)
				}
			}
		}
		if !known {
			return PlayerAttentionView{}, ErrAttentionUnknown
		}
		if len(*ids) >= PlayerAttentionLimit {
			return PlayerAttentionView{}, ErrAttentionLimit
		}
		*ids = append(*ids, in.ID)
	} else {
		*ids = append((*ids)[:index:index], (*ids)[index+1:]...)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	eventID := newID("attention")
	data, _ := json.Marshal(attention)
	var value map[string]any
	_ = json.Unmarshal(data, &value)
	delta := StateDeltaEvent{V: schemaVersion, Type: StoryEventTypeStateDelta, ID: eventID, ParentID: branch.Head, BranchID: in.BranchID, Ts: now, SchemaVersion: stateOpSchemaVersion, Ops: []interactivestate.Op{{Op: "set", Path: playerAttentionRoot, Value: value, SourceKind: "player_attention", SourceID: eventID, Reason: "Player changed a reading preference"}}}
	branch.Head = eventID
	meta.Branches[in.BranchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, delta); err != nil {
		return PlayerAttentionView{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return PlayerAttentionView{}, err
	}
	slog.Info("Player attention updated", "story_id", storyID, "branch_id", in.BranchID, "kind", in.Kind, "target_id", in.ID, "following", in.Following)
	return PlayerAttentionView{attention, eventID}, nil
}
