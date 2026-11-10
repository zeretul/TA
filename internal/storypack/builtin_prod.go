//go:build embedweb

package storypack

// devBuiltinPacks is empty in a delivered build. See builtin_dev.go: the
// detective pack is a test world and is not part of what a player receives.
func devBuiltinPacks() []string {
	return nil
}
