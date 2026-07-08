package file

import (
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// lockFileName is the per-scope-directory advisory lockfile used to serialize
// read-modify-write cycles across processes.
const lockFileName = ".lock"

// lock acquires the process-wide mutex and, best-effort, an exclusive OS file
// lock (flock) on the scope's lockfile, then returns the release function
// (intended for `defer s.lock()()`).
//
// The file lock makes a read-file → modify → write-file cycle atomic across
// concurrent processes (two CLI invocations, or the CLI and the GUI) — without
// it, the process-wide mutex alone let two processes interleave and the last
// writer silently dropped the other's newly staged entry. A rare failure to
// create or acquire the file lock degrades to process-mutex-only rather than
// failing the operation.
func (s *Store) lock() func() {
	fileMu.Lock()

	lockPath := s.lockPath()

	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil { //nolint:mnd // owner-only directory
		return fileMu.Unlock
	}

	fl := flock.New(lockPath)
	if err := fl.Lock(); err != nil {
		return fileMu.Unlock
	}

	return func() {
		_ = fl.Unlock()
		fileMu.Unlock()
	}
}

// lockPath returns the advisory lockfile path: the scope directory's .lock in
// split (working) mode, or a sibling .lock next to the state file in
// single-file mode.
func (s *Store) lockPath() string {
	if s.isSplit() {
		return filepath.Join(s.stateDir, lockFileName)
	}

	return filepath.Join(filepath.Dir(s.stateFilePath), lockFileName)
}
