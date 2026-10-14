package snapshot

import (
	"os"
	"path/filepath"
	"time"
)

// WriteFile atomically replaces path with a snapshot whose records are
// produced by fill. The snapshot is written to a temporary file in the
// same directory and renamed over path once complete, so a reader never
// observes a partially written snapshot.
//
// It returns the number of records written and the size of the file.
func WriteFile(path string, created time.Time, fill func(*Encoder) error) (records uint64, size int64, err error) {
	f, err := os.CreateTemp(filepath.Dir(path), ".snapshot-*.tmp")
	if err != nil {
		return 0, 0, err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(tmp)
		}
	}()

	enc := NewEncoder(f, created)
	if err = fill(enc); err != nil {
		return 0, 0, err
	}
	if err = enc.Close(); err != nil {
		return 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}
	if err = f.Close(); err != nil {
		return 0, 0, err
	}
	if err = os.Rename(tmp, path); err != nil {
		return 0, 0, err
	}
	return enc.Count(), info.Size(), nil
}
