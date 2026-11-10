package interactive

func adventureWeaponClassValid(value string) bool {
	switch value {
	case "blade", "sword", "staff", "spear", "fist", "bow", "pistol", "rifle":
		return true
	}
	return false
}

// The actual equipped item supplies weapon identity and class. A genuine local
// adaptation retains its source; a name or an unequipped gun grants neither.
func adventureSkillWeaponAvailable(r AdventureRules, skill AdventureSkill, inventory map[string]int, equipment map[string]string) bool {
	if skill.RequiredWeapon == "" && skill.RequiredWeaponClass == "" {
		return true
	}
	id := equipment[skill.Scale+":weapon"]
	if inventory[id] < 1 {
		return false
	}
	for _, item := range r.Items {
		if item.ID != id || item.Kind != "weapon" || item.Scale != skill.Scale {
			continue
		}
		exact := skill.RequiredWeapon == "" || id == skill.RequiredWeapon || item.GeneratedFrom == skill.RequiredWeapon
		class := skill.RequiredWeaponClass == "" || item.WeaponClass == skill.RequiredWeaponClass
		return exact && class
	}
	return false
}
