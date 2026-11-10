package app

import (
	"context"
	"fmt"
	"strings"

	"ta/internal/interactive"
)

// The Store identity fences workspace changes. Branch is deliberately absent:
// an empty branch means the current branch, and both spellings of one command
// must wait on the same in-flight decision before looking for its receipt.
type enemyDecisionCommandKey struct {
	store     *interactive.Store
	storyID   string
	commandID string
}

// acquireEnemyDecisionCommand serializes concurrent retries without holding a
// Store lock or allocating a goroutine per waiter. Completed entries are removed;
// canceled waiters leave no references or goroutines behind. This is an in-flight
// gate, not a replacement for durable journal idempotency.
func (a *App) acquireEnemyDecisionCommand(ctx context.Context, store *interactive.Store, storyID, commandID string) (func(), error) {
	key := enemyDecisionCommandKey{store: store, storyID: strings.TrimSpace(storyID), commandID: strings.TrimSpace(commandID)}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		a.mu.Lock()
		if a.enemyDecisionFlights == nil {
			a.enemyDecisionFlights = make(map[enemyDecisionCommandKey]chan struct{})
		}
		done, busy := a.enemyDecisionFlights[key]
		if !busy {
			done = make(chan struct{})
			a.enemyDecisionFlights[key] = done
			a.mu.Unlock()
			return func() {
				a.mu.Lock()
				delete(a.enemyDecisionFlights, key)
				close(done)
				a.mu.Unlock()
			}, nil
		}
		a.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-done:
		}
	}
}

func (a *App) resolveBattleWithEnemyDecision(ctx context.Context, store *interactive.Store, storyID string, request interactive.BattleRoundRequest) (interactive.BattleRoundOutcome, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	release, err := a.acquireEnemyDecisionCommand(ctx, store, storyID, request.CommandID)
	if err != nil {
		return interactive.BattleRoundOutcome{}, err
	}
	defer release()
	// Re-check after waiting: the previous holder may have committed the answer.
	prepared, err := store.PrepareBattleEnemyDecision(storyID, request)
	if err != nil {
		return interactive.BattleRoundOutcome{}, err
	}
	if prepared.Replayed {
		if prepared.Battle == nil {
			return interactive.BattleRoundOutcome{}, fmt.Errorf("combat decision replay is missing its committed outcome")
		}
		return *prepared.Battle, nil
	}
	request.BranchID = strings.TrimSpace(prepared.Snapshot.BranchID)
	if request.BranchID == "" {
		return interactive.BattleRoundOutcome{}, fmt.Errorf("combat decision snapshot is missing its branch")
	}
	choices, status, err := a.decideEnemyActions(ctx, store, storyID, request.BranchID, prepared.Snapshot)
	if err != nil {
		return interactive.BattleRoundOutcome{}, err
	}
	request.ExpectedHead = prepared.Snapshot.Head
	request.ExpectedRound = prepared.Snapshot.Round
	request.EnemyChoices = choices
	request.DecisionStatus = status
	if err := ctx.Err(); err != nil {
		return interactive.BattleRoundOutcome{}, err
	}
	return store.ResolveBattleRound(storyID, request)
}

func (a *App) commitAdventureWithEnemyDecision(ctx context.Context, store *interactive.Store, storyID, branchID, commandID, head string, input interactive.AdventureAction) (interactive.AdventureActionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	release, err := a.acquireEnemyDecisionCommand(ctx, store, storyID, commandID)
	if err != nil {
		return interactive.AdventureActionResult{}, err
	}
	defer release()
	prepared, err := store.PrepareAdventureEnemyDecision(storyID, branchID, commandID, head, input)
	if err != nil {
		return interactive.AdventureActionResult{}, err
	}
	if prepared.Replayed {
		if prepared.Adventure == nil {
			return interactive.AdventureActionResult{}, fmt.Errorf("adventure decision replay is missing its committed outcome")
		}
		return *prepared.Adventure, nil
	}
	// Resolve the current-branch alias once. Creating/switching a fork during
	// Generate may preserve the same head but must not redirect this command.
	branchID = strings.TrimSpace(prepared.Snapshot.BranchID)
	if branchID == "" {
		return interactive.AdventureActionResult{}, fmt.Errorf("adventure decision snapshot is missing its branch")
	}
	choices, status, err := a.decideEnemyActions(ctx, store, storyID, branchID, prepared.Snapshot)
	if err != nil {
		return interactive.AdventureActionResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return interactive.AdventureActionResult{}, err
	}
	return store.CommitAdventureActionWithEnemyDecision(storyID, branchID, commandID, head, input, choices, status)
}
