package storypack

import _ "embed"

// Candidate worlds retain isolated IDs and authored, revision-scoped data.
// These assets require the generic authored-state and lore-audience consumers.

//go:embed assets/doupo-continent-ta07-full-gm-candidate.json
var builtinRuleCandidate0 string

//go:embed assets/harry-potter-1991-student-ta07-full-gm-candidate.json
var builtinRuleCandidate1 string

//go:embed assets/harry-potter-1991-ta07-full-gm-candidate.json
var builtinRuleCandidate2 string

//go:embed assets/lord-of-mysteries-ta07-full-gm-candidate.json
var builtinRuleCandidate3 string

//go:embed assets/sanguo-190-ta07-full-gm-candidate.json
var builtinRuleCandidate4 string

//go:embed assets/zichuan-xichuan-ta07-full-gm-candidate.json
var builtinRuleCandidate5 string

//go:embed assets/doupo-continent-ta07-full-gm-candidate.en-US.json
var builtinRuleCandidateEnglish0 string

//go:embed assets/harry-potter-1991-student-ta07-full-gm-candidate.en-US.json
var builtinRuleCandidateEnglish1 string

//go:embed assets/harry-potter-1991-ta07-full-gm-candidate.en-US.json
var builtinRuleCandidateEnglish2 string

//go:embed assets/lord-of-mysteries-ta07-full-gm-candidate.en-US.json
var builtinRuleCandidateEnglish3 string

//go:embed assets/zichuan-xichuan-ta07-full-gm-candidate.en-US.json
var builtinRuleCandidateEnglish4 string
