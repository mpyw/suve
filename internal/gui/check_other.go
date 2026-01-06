//go:build (production || dev) && !linux

package gui

// checkGUIDependencies is a no-op on non-Linux platforms.
// macOS and Windows handle their dependencies differently:
// - macOS: WebKit is included in the OS
// - Windows: WebView2 runtime is auto-installed if missing
func checkGUIDependencies() error {
	return nil
}
