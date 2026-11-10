package loreapp

import (
	"context"

	booklore "ta/internal/book/lore"
)

// ClassifyItems runs semantic classification with the selected Project's
// settings and durable Tool Agent session. It never consults the foreground
// Book, so imports can safely target a background AgentChat Project.
func (service *Service) ClassifyItems(ctx context.Context, projectID string, inputs []booklore.ClassificationInput) ([]booklore.ClassificationSuggestion, error) {
	if service == nil || service.host == nil {
		return nil, ErrNoWorkspace
	}
	return service.host.ClassifyLoreItems(ctx, projectID, inputs)
}
