//go:build !production && !dev

package main

// runGUIIfRequested is a no-op when GUI is not available.
func runGUIIfRequested() bool {
	return false
}

// registerGUIFlag is a no-op when GUI is not available.
func registerGUIFlag() {}
