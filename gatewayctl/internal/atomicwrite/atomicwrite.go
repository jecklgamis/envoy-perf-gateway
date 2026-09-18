// Package atomicwrite writes a file via a temp file + rename, so a reader
// (Envoy's inotify watch, or config_server serving it) never observes a
// missing or partially-written file mid-swap.
package atomicwrite

import (
	"os"
	"path/filepath"
)

func Write(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}
