package fs

import (
	"os"
	"path/filepath"
)

var _ FileSystem = (*osFileSystem)(nil)

// osFileSystem is a concrete implementation of the FileSystem interface, providing
// basic file operations by using standard library functions from the os and filepath packages.
// This implementation is thread-safe for its methods, allowing concurrent access for
// creating, opening, reading, and joining file paths.
type osFileSystem struct{}

// NewOsFileSystem creates a new instance of osFileSystem, providing an implementation of
// the FileSystem interface using the standard os package.
// This function does not require any configuration and returns a pointer to an osFileSystem.
func NewOsFileSystem() *osFileSystem {
	return &osFileSystem{}
}

// Open opens the file specified by its name and returns a File interface for interacting
// with the file's contents. It utilizes os.Open from the standard library.
func (o *osFileSystem) Open(name string) (File, error) {
	return os.Open(name)
}

// ReadDir reads the contents of the directory specified by the name and returns a slice
// of os.DirEntry, representing the files and directories within.
func (o *osFileSystem) ReadDir(name string) ([]os.DirEntry, error) {
	return os.ReadDir(name)
}

// Join joins any number of path elements into a single path using filepath.Join from the
// standard library.
func (o *osFileSystem) Join(elem ...string) string {
	return filepath.Join(elem...)
}
