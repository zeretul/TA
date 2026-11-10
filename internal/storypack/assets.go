package storypack

import _ "embed"

// The formal 凡人人界 pack is maintained in the tracked JSON asset below.
// It is the author-content source for this branch, including the character
// catalog, world-book bodies and creation choices. Historical profile files and
// the old untracked catalog generators are provenance, not a supported rebuild
// command: running an old generator would discard later authored changes.
//
// Edit this asset with its source evidence, increment content_version when the
// content changes, and run the existing catalog and reference checks. The
// maintenance record in work/worldbook-revision-merge.md explains the retained
// profile sections and the author-review amendments. Story-bound old revisions
// remain frozen; changing this asset does not rewrite their saved content.
//
//go:embed assets/fanren-jie.json
var builtinPackJSONFanrenFormal string

// The English display text for the pack above, keyed by the same stable ids.
// It carries prose only — no numbers, no rule tokens, no ids — and is mounted
// as Localizations["en-US"]. Keeping it in its own asset means the Chinese
// source stays the single author-content record: a translation can be revised
// without touching a fact, and a world shipped without one simply reads in the
// language its author wrote.
//
//go:embed assets/fanren-jie.en-US.json
var builtinPackLocalizationFanrenEN string

// The two authored adventure worlds retain their manuscript provenance in
// scripts/build-astra-worlds.py and the batch's source-hashes evidence.
//
//go:embed assets/lanchuan-qunxia.json
var builtinPackJSONLanchuan string

//go:embed assets/xinghai-689.json
var builtinPackJSONXinghai string

//go:embed assets/lanchuan-qunxia.en-US.json
var builtinPackLocalizationLanchuanEN string

//go:embed assets/xinghai-689.en-US.json
var builtinPackLocalizationXinghaiEN string

//go:embed assets/sui-guan-realms.json
var builtinPackJSONShatteredCrown string

//go:embed assets/sui-guan-realms.en-US.json
var builtinPackLocalizationShatteredCrownEN string
