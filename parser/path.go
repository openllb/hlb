package parser

import (
	"path/filepath"

	homedir "github.com/mitchellh/go-homedir"
)

func ResolvePath(node Node, path string) (string, error) {
	path, err := homedir.Expand(path)
	if err != nil {
		return path, err
	}

	if filepath.IsAbs(path) {
		return path, nil
	}

	return filepath.Join(filepath.Dir(node.Position().Filename), path), nil
}
