//go:build !production && !dev

// Package main provides the suve CLI entry point.
package main

// registerGUIFlag is a no-op when GUI is not available.
func registerGUIFlag() {}

// registerGUIDescription is a no-op when GUI is not available.
func registerGUIDescription() {}
