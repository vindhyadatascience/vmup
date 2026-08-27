//go:build !darwin

package selfupdate

// isTranslated is meaningful only on darwin; every other platform reports its
// real architecture.
func isTranslated() bool { return false }
