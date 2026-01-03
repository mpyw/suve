//go:build darwin && (production || dev)

package gui

// #cgo darwin LDFLAGS: -framework UniformTypeIdentifiers
import "C"
