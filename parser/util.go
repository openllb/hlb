package parser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alecthomas/participle/v2/lexer"
)

// ResolvePath resolves the path relative to root, and expands `~` to the
// user's home directory.
func ResolvePath(root, path string) (string, error) {
	path, err := ExpandHomeDir(path)
	if err != nil {
		return path, err
	}
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Join(root, path), nil
}

// ExpandHomeDir expands the path to include the home directory if the path is
// prefixed with `~`. If it isn't prefixed with `~`, the path is returned as-is.
func ExpandHomeDir(path string) (string, error) {
	if len(path) == 0 {
		return path, nil
	}

	if path[0] != '~' {
		return path, nil
	}

	if len(path) > 1 && path[1] != '/' && path[1] != '\\' {
		return "", errors.New("cannot expand user-specific home dir")
	}

	// Works without cgo, available since go1.12
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, path[1:]), nil
}

// FormatPos returns a lexer.Position formatted as a string.
func FormatPos(pos lexer.Position) string {
	return fmt.Sprintf("%s:%d:%d:", pos.Filename, pos.Line, pos.Column)
}
