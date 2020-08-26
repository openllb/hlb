package llbutil

import (
	"fmt"
	"os"
	"path/filepath"
)

type IncludePatterns struct {
	Patterns []string
}

type ExcludePatterns struct {
	Patterns []string
}

func FilterLocalFiles(localPath string, includePatterns, excludePatterns []string) (localPaths []string, err error) {
	var fi os.FileInfo
	fi, err = os.Stat(localPath)
	if err != nil {
		return
	}

	switch {
	case fi.Mode().IsRegular():
		localPaths = append(localPaths, localPath)
		return
	case fi.Mode().IsDir():
		err = filepath.Walk(localPath, func(walkPath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			relPath, err := filepath.Rel(localPath, walkPath)
			if err != nil {
				return err
			}
			if relPath == "." {
				return nil
			}
			if len(includePatterns) > 0 {
				for _, pattern := range includePatterns {
					if ok, err := filepath.Match(pattern, relPath); ok && err == nil {
						if info.Mode().IsRegular() {
							localPaths = append(localPaths, walkPath)
						}
						return nil
					} else if err != nil {
						return err
					}
				}
				// Didn't match include, so skip directory.
				if info.Mode().IsDir() {
					return filepath.SkipDir
				}
				return nil
			} else if len(excludePatterns) > 0 {
				for _, pattern := range excludePatterns {
					if ok, err := filepath.Match(pattern, relPath); !ok && err == nil {
						if info.Mode().IsDir() {
							return filepath.SkipDir
						}
						return nil
					} else if err != nil {
						return err
					}
				}
				// Didn't match exclude to add it to list.
				if info.Mode().IsRegular() {
					localPaths = append(localPaths, walkPath)
				}
				return nil
			}
			if info.Mode().IsRegular() {
				localPaths = append(localPaths, walkPath)
			}
			return nil
		})
		return
	default:
		return localPaths, fmt.Errorf("unexpected file type at %s", localPath)
	}
}
