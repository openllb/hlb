package llbutil

import (
	"os"
	"path/filepath"
	"time"

	"github.com/tonistiigi/fsutil/types"
)

type FileInfo struct {
	*types.Stat
}

func (fi *FileInfo) Name() string {
	return filepath.Base(fi.Stat.Path)
}

func (fi *FileInfo) Size() int64 {
	return fi.Stat.Size_
}

func (fi *FileInfo) Mode() os.FileMode {
	return os.FileMode(fi.Stat.Mode)
}

func (fi *FileInfo) ModTime() time.Time {
	return time.Unix(fi.Stat.ModTime/1e9, fi.Stat.ModTime%1e9)
}

func (fi *FileInfo) IsDir() bool {
	return fi.Mode().IsDir()
}

func (fi *FileInfo) Sys() interface{} {
	return fi.Stat
}
