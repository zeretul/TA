package interactive

// Check the actual branch inventory, including quantities after a sale. Holding
// a name, an author definition or a source sentence is not holding the book.
func cultivationManualHeld(state map[string]any, actorID, defID string) bool {
	if defID == "" {
		return true
	}
	items, _ := xiuxianEquipmentEntry(state, actorID, xiuxianFieldImportantItems)
	for _, raw := range items {
		entry, _ := raw.(map[string]any)
		if xiuxianEntryText(entry, "定义") == defID && xiuxianCarriedQuantity(entry) > 0 {
			return true
		}
	}
	return false
}
