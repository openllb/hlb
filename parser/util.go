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

func IsPositionWithinNode(node Node, line, column int) bool {
	return IsIntersect(node.Position(), node.End(), line, column)
}

func IsIntersect(start, end lexer.Position, line, column int) bool {
	if start.Column == 0 || end.Column == 0 || column == 0 {
		return line >= start.Line && line <= end.Line
	}
	if (line < start.Line || line > end.Line) ||
		(line == start.Line && column < start.Column) ||
		(line == end.Line && column >= end.Column) {
		return false
	}
	return true
}
