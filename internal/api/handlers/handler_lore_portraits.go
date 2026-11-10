package handlers

import booklore "ta/internal/book/lore"

// loreItemResponse is one world-book entry as the author's library reads it: the
// stored entry, plus the shipped default portrait of the person the entry is
// about when the author has not chosen a picture of their own.
//
// The two portrait fields are derived on every read and never stored. They are
// reported separately from the entry's own image so the client can tell "this
// entry has a picture the author can clear" from "this person simply has a
// default face", which is what keeps the clear control honest.
type loreItemResponse struct {
	booklore.Item
	PortraitURL        string `json:"portrait_url,omitempty"`
	PortraitDisplayURL string `json:"portrait_display_url,omitempty"`
}

// withLorePortraits decorates entries with their default portraits. A read
// failure or a world with no catalog leaves the entries exactly as stored; a
// missing portrait is an ordinary answer, not an error the page should show.
func (h *Handlers) withLorePortraits(projectID string, items []booklore.Item) []loreItemResponse {
	if items == nil {
		return nil
	}
	out := make([]loreItemResponse, 0, len(items))
	for _, item := range items {
		out = append(out, loreItemResponse{Item: item})
	}
	portraits := h.app.LoreItemPortraitURLs(projectID, items)
	if len(portraits) == 0 {
		return out
	}
	for index := range out {
		if portrait, ok := portraits[out[index].Item.ID]; ok {
			out[index].PortraitURL = portrait.ThumbnailURL
			out[index].PortraitDisplayURL = portrait.DisplayURL
		}
	}
	return out
}

// withLorePortrait decorates a single entry, for the write paths that answer
// with the entry they just stored.
func (h *Handlers) withLorePortrait(projectID string, item booklore.Item) loreItemResponse {
	decorated := h.withLorePortraits(projectID, []booklore.Item{item})
	if len(decorated) == 0 {
		return loreItemResponse{Item: item}
	}
	return decorated[0]
}
