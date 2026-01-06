//go:build !production && !dev

package main

// registerGUIFlag is a no-op when GUI is not available.
func registerGUIFlag() {}

// registerGUIDescription is a no-op when GUI is not available.
func registerGUIDescription() {}
