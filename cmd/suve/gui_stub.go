//go:build !production && !dev

// Package main provides the suve CLI entry point.
//
// gui_stub.go holds the no-op registration hooks main.go calls in a build
// without the GUI. It is a working part of main, so it joins main's namespace.
//
//declscope:namespace main
package main

// registerGUIFlag is a no-op when GUI is not available.
func registerGUIFlag() {}

// registerGUIDescription is a no-op when GUI is not available.
func registerGUIDescription() {}
