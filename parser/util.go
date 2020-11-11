package parser

import (
	"fmt"
	"path/filepath"

	"github.com/alecthomas/participle/lexer"
	homedir "github.com/mitchellh/go-homedir"
)

func ResolvePath(root, path string) (string, error) {
	path, err := homedir.Expand(path)
	if err != nil {
		return path, err
	}
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Join(root, path), nil
}

// FormatPos returns a lexer.Position formatted as a string.
func FormatPos(pos lexer.Position) string {
	return fmt.Sprintf("%s:%d:%d:", pos.Filename, pos.Line, pos.Column)
}
