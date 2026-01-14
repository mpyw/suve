//go:build windows

package security

import (
	"golang.org/x/sys/windows"
)

// SetupProcess configures Windows-specific security measures.
func SetupProcess() error {
	// Set error mode to prevent crash dialogs and Windows Error Reporting
	// from generating crash dumps that might contain sensitive data.
	//
	// SEM_FAILCRITICALERRORS: Don't display critical error dialogs
	// SEM_NOGPFAULTERRORBOX: Don't display GP fault error box (prevents WER)
	// SEM_NOOPENFILEERRORBOX: Don't display file error dialogs
	windows.SetErrorMode(windows.SEM_FAILCRITICALERRORS | windows.SEM_NOGPFAULTERRORBOX | windows.SEM_NOOPENFILEERRORBOX)

	return nil
}
