package fs

import (
	"io"
	"os"
)

//go:generate mockgen -source=filesystem.go -destination=../../pkg/mocks/fs_mock.go -package=mocks os DirEntry
//go:generate mockgen -destination=../../pkg/mocks/os_mock.go -package=mocks os DirEntry

// FileSystem is an interface for file operations that provides essential methods
// to open files, read directory contents, and join paths.
// This interface guarantees thread-safe access to its methods, as multiple goroutines
// may concurrently request filesystem resources. However, it does not guarantee
// thread-safe access to the files themselves; concurrent access to a single file
// must be handled externally if required.
// Implementations of FileSystem may throw panics during filesystem interactions;
// the calling code should handle or recover from these panics as necessary.
type FileSystem interface {
	// Open opens the file specified by its name and returns a File interface
	// instance for reading file contents.
	// This method itself is thread-safe, allowing multiple goroutines to request
	// file access concurrently, but the returned File instance does not guarantee
	// thread-safe operations. Any concurrent access to the file should be handled
	// by the calling code.
	// Panics may be thrown if the file cannot be accessed or if a critical issue
	// arises; the calling context should handle or recover from such panics.
	Open(name string) (File, error)

	// ReadDir reads the contents of the specified directory name and returns
	// a slice of os.DirEntry representing files and directories in that directory.
	// This method is thread-safe and can be accessed concurrently by multiple goroutines.
	// Panics may occur due to critical issues during the read operation, and these
	// should be handled by the calling context.
	ReadDir(name string) ([]os.DirEntry, error)

	// Join joins any number of path elements into a single path. This method is
	// thread-safe as it operates on string concatenation and does not directly access
	// shared resources. It allows safe concurrent path generation by multiple goroutines.
	Join(elem ...string) string
}

// File represents a file interface that provides both reading and closing capabilities.
// It embeds io.ReadCloser, inheriting read and close methods. Note that the interface
// itself does not guarantee thread-safe access to the underlying file's contents.
// Concurrent access to an instance of File should be managed by the calling code.
type File interface {
	io.ReadCloser
}
