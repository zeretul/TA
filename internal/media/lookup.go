package media

import "strings"

// bindingScore is the shared identity and visibility check for stills and clips.
// A scene-specific asset wins before an ordinary one; within that level the
// world's own asset wins before an explicitly shared fallback.
//
// An entry nobody has looked at is not a candidate at all. The manifest's tags
// say where a file was meant to go, not what is actually in it, so matching
// alone must never be enough to put a file in front of a player.
func bindingScore(asset Asset, query PictureQuery) int {
	if !asset.Reviewed && !(asset.isActivityClip() && asset.PlaybackApproval == "local_owner") {
		return -1
	}
	role := strings.TrimSpace(query.Role)
	if role == "" {
		role = RoleGeneral
	}
	if asset.Purpose != query.Purpose || asset.Role != role {
		return -1
	}
	world := normalizeWorld(query.World)
	if asset.World != "" && asset.World != world {
		return -1
	}
	if place := normalizePlace(query.Place); place != "" {
		if asset.Subject != SubjectPlace || asset.Place != place {
			return -1
		}
		// A place the world pack registered names no scope and answers every
		// story in that world. A place one story invented names that story, and a
		// query from a sibling story must not draw it: the two stories made up the
		// same name independently.
		if asset.Scope != "" && asset.Scope != strings.TrimSpace(query.Scope) {
			return -1
		}
	} else {
		if asset.Subject != SubjectCharacter {
			return -1
		}
		if asset.SourceKey != "" {
			if asset.SourceKey != strings.TrimSpace(query.SourceKey) {
				return -1
			}
		} else if asset.Scope == "" || asset.Actor == "" || asset.Scope != strings.TrimSpace(query.Scope) || asset.Actor != strings.TrimSpace(query.Actor) {
			return -1
		}
	}
	score := 0
	if asset.World != "" {
		score++
	}
	if asset.Scene != "" {
		if asset.Scene != strings.TrimSpace(query.SceneKey) {
			return -1
		}
		score += 2
	}
	return score
}

// ClipsFor applies the same world, actor, scene and activity-role checks as a
// picture. Legacy source-key-only lookups are not used by player surfaces.
func (l *Library) ClipsFor(query PictureQuery) []Asset {
	query.Purpose = PurposeClip
	l.load()
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.loadErr != nil {
		return nil
	}
	bestScore := -1
	var result []Asset
	for _, asset := range l.entries {
		score := bindingScore(asset, query)
		if score < 0 || score < bestScore {
			continue
		}
		if score > bestScore {
			result = nil
			bestScore = score
		}
		result = append(result, asset)
	}
	return result
}
