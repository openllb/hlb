package llbutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tonistiigi/fsutil"
)

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
		opt := &fsutil.WalkOpt{
			IncludePatterns: includePatterns,
			ExcludePatterns: excludePatterns,
		}
		err = fsutil.Walk(context.TODO(), localPath, opt, func(walkPath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				localPaths = append(localPaths, filepath.Join(localPath, walkPath))
			}
			return nil
		})
		return
	default:
		return localPaths, fmt.Errorf("unexpected file type at %s", localPath)
	}
}
