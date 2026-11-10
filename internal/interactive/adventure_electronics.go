package interactive

// A personal near-field discharge may hit a registered local electronic node
// during a machine encounter. This is not a general cross-scale attack path.
func AdventureElectronicTargetInRange(s AdventureSkill, b *AdventureBattle) bool {
	if s.Scale != "person" || len(s.Effects) != 1 || s.Effects[0].Kind != "electronic_disrupt" || b == nil || b.Scale == "ship" || b.ElectronicDistanceMeters < 1 || b.ElectronicDistanceMeters > s.RangeMeters {
		return false
	}
	for _, effect := range s.Effects {
		if effect.Kind == "electronic_disrupt" {
			return true
		}
	}
	return false
}
