//go:build !production && !dev

// Package main provides the suve CLI entry point.
//
// gui_stub.go holds the no-op registration hooks main.go calls in a build
// without the GUI. It stands in for gui.go, so it shares gui.go's namespace.
//
//declscope:namespace gui
package main

// registerGUIFlag is a no-op when GUI is not available.
//
//declscope:package // called from main.go
func registerGUIFlag() {}

// registerGUIDescription is a no-op when GUI is not available.
//
//declscope:package // called from main.go
func registerGUIDescription() {}
