package store

import "os"

// writeBad overwrites a snapshot with bytes that fail gzip decoding.
func writeBad(path string) error { return os.WriteFile(path, []byte("not a gzip stream"), 0o644) }

// statFile is os.Stat with the error returned as-is.
func statFile(path string) (os.FileInfo, error) { return os.Stat(path) }
