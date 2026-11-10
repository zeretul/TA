package interactive

import (
	"encoding/json"
	"fmt"
	"ta/internal/cultivation"
)

// Progress is part of the actor's canonical journal state, never a second save.
// Inputs are paid when a batch starts; its fixed outcome is released only when
// the remaining work is finished. The narrator sees progress, not the draw.
const craftWorkField = "制作进度"

type craftWork struct {
	MethodVersion int                        `json:"method_version"`
	WorkedMinutes int64                      `json:"worked_minutes"`
	Products      []cultivation.CraftProduct `json:"products,omitempty"`
	FailedBatches int                        `json:"failed_batches,omitempty"`
}

func craftWorkFor(state map[string]any, actorID, methodID string) (craftWork, bool) {
	entries, _ := xiuxianEquipmentEntry(state, actorID, craftWorkField)
	var work craftWork
	raw, ok := entries[methodID]
	if !ok {
		return work, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil || json.Unmarshal(encoded, &work) != nil {
		return craftWork{WorkedMinutes: -1}, true
	}
	return work, true
}

func planCraftWork(registry *cultivation.CraftRegistry, state map[string]any, actorID string, method cultivation.CraftMethodDef, request cultivation.CraftRequest) (cultivation.CraftBatch, map[string]any, error) {
	entries, _ := xiuxianEquipmentEntry(state, actorID, craftWorkField)
	next := make(map[string]any, len(entries)+1)
	for key, value := range entries {
		next[key] = value
	}
	if request.Minutes < 1 {
		return cultivation.CraftBatch{}, nil, cultivation.ErrCraftDurationTooShort
	}
	total := cultivation.CraftBatch{}
	remaining := request.Minutes
	if pending, ok := craftWorkFor(state, actorID, method.ID); ok {
		if pending.MethodVersion != method.Version || pending.WorkedMinutes < 1 || pending.WorkedMinutes >= method.MinutesPerBatch {
			return total, nil, fmt.Errorf("craft: invalid saved work")
		}
		// Resuming still needs the actual qualification, location and reusable tools.
		if !stageReaches(request.Stage, method.MinimumStage) {
			return total, nil, cultivation.ErrCraftStageTooLow
		}
		if method.RequiredLocation != "" && request.Location != method.RequiredLocation {
			return total, nil, fmt.Errorf("craft: required location unavailable")
		}
		for _, tool := range method.RequiredTools {
			found := false
			for _, held := range request.Held {
				found = found || (held.DefID == tool && held.Quantity > 0)
			}
			if !found {
				return total, nil, fmt.Errorf("craft: required tool unavailable: %s", tool)
			}
		}
		used := min(remaining, method.MinutesPerBatch-pending.WorkedMinutes)
		remaining -= used
		total.ElapsedMinutes += used
		pending.WorkedMinutes += used
		if pending.WorkedMinutes < method.MinutesPerBatch {
			next[method.ID] = pending
			return total, next, nil
		}
		total.Batches++
		total.Products = append(total.Products, pending.Products...)
		total.FailedBatches += pending.FailedBatches
		delete(next, method.ID)
	}
	if remaining == 0 {
		return total, next, nil
	}
	// Round up solely to reserve materials for the final unfinished batch.
	// The clock below advances only by the actually authorized minutes.
	allocation := request
	allocation.Minutes = ((remaining-1)/method.MinutesPerBatch + 1) * method.MinutesPerBatch
	batch, err := cultivation.PlanCraftBatch(registry, allocation)
	if err != nil {
		if total.ElapsedMinutes > 0 {
			return total, next, nil
		}
		return total, nil, err
	}
	used := min(remaining, batch.ElapsedMinutes)
	if partial := used % method.MinutesPerBatch; partial > 0 {
		work := craftWork{MethodVersion: method.Version, WorkedMinutes: partial}
		if batch.FailedBatches > 0 {
			work.FailedBatches = 1
			batch.FailedBatches--
		} else {
			for i := range batch.Products {
				count := batch.Products[i].Quantity / batch.Batches
				work.Products = append(work.Products, cultivation.CraftProduct{DefID: batch.Products[i].DefID, Quantity: count})
				batch.Products[i].Quantity -= count
			}
		}
		batch.Batches--
		next[method.ID] = work
	}
	total.ElapsedMinutes += used
	total.Batches += batch.Batches
	total.FailedBatches += batch.FailedBatches
	total.Consumed = append(total.Consumed, batch.Consumed...)
	for _, p := range batch.Products {
		if p.Quantity > 0 {
			total.Products = append(total.Products, p)
		}
	}
	return total, next, nil
}
