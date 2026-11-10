package buildinfo

// Version is injected by release builds. The default keeps source/dev runs from
// treating themselves as installable release binaries.
var Version = "dev"

// Repository is the upstream project this build derives from. It stays
// readable for attribution and for locating the baseline this fork started
// from; it is deliberately not the update source.
const Repository = "alfredxw/denova"

// UpstreamUpdatesEnabled reports whether this build may fetch and install a
// release from Repository.
//
// It is false for this distribution: the game build is produced from a fork
// and ships on its own channel, so an upstream release is a different product
// rather than a newer version of this one. Applying it would replace the game
// build with the upstream creation tool. The update chain therefore refuses at
// the source instead of relying on hidden buttons, and the upstream baseline
// stays inspectable through Repository and the local version above.
const UpstreamUpdatesEnabled = false

// UpdateRepository is set only by the release builder, as owner/repository.
// An empty value disables online updates; Repository remains attribution only.
var UpdateRepository = ""
