// Package style owns narrative-style identity and the user-managed reference
// library. Prompt content remains with the interactive Teller library.
package style

// DefaultID is the style a new story starts on. It is the design's default
// ("朴实正剧"), not the pre-migration rhythm style: a story that never chose a
// style should open on the recommended set's first entry rather than on a
// legacy voice the picker no longer leads with.
const DefaultID = "source-drama"
