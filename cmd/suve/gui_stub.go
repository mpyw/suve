//go:build !production && !dev

// Package main provides the suve CLI entry point.
//
// gui_stub.go は main.go が呼ぶ登録フックの no-op 実装（GUI 無し build 側）で、
// main の作業部品なので namespace を main に合流する。
//
//declscope:namespace main
package main

// registerGUIFlag is a no-op when GUI is not available.
func registerGUIFlag() {}

// registerGUIDescription is a no-op when GUI is not available.
func registerGUIDescription() {}
