package llbutil

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterLocalFiles(t *testing.T) {
	localPath, err := os.MkdirTemp("", "test")
	require.NoError(t, err)
	files := []string{"decrypted/secret", "other/decrypted/secret", "secret", "src/foo"}
	for _, f := range files {
		err = os.MkdirAll(filepath.Dir(filepath.Join(localPath, f)), 0755)
		require.NoError(t, err)
		fs, err := os.Create(filepath.Join(localPath, f))
		require.NoError(t, err)
		fs.Close()
	}

	got, err := FilterLocalFiles(localPath, nil, nil)
	require.NoError(t, err)
	relativeFiles(localPath, got)
	require.Equal(t, files, got)

	got, err = FilterLocalFiles(localPath, []string{"**/nada"}, nil)
	require.NoError(t, err)
	relativeFiles(localPath, got)
	require.Nil(t, got)

	got, err = FilterLocalFiles(localPath, []string{"secret"}, nil)
	require.NoError(t, err)
	relativeFiles(localPath, got)
	require.Equal(t, []string{"secret"}, got)

	got, err = FilterLocalFiles(localPath, []string{"*/secret"}, nil)
	require.NoError(t, err)
	relativeFiles(localPath, got)
	require.Equal(t, []string{"decrypted/secret"}, got)

	got, err = FilterLocalFiles(localPath, []string{"**/secret"}, nil)
	require.NoError(t, err)
	relativeFiles(localPath, got)
	require.Equal(t, []string{"decrypted/secret", "other/decrypted/secret", "secret"}, got)

	got, err = FilterLocalFiles(localPath, []string{"**/decrypted"}, nil)
	require.NoError(t, err)
	relativeFiles(localPath, got)
	require.Equal(t, []string{"decrypted/secret", "other/decrypted/secret"}, got)

	got, err = FilterLocalFiles(localPath, []string{"**/decrypted"}, []string{"other"})
	require.NoError(t, err)
	relativeFiles(localPath, got)
	require.Equal(t, []string{"decrypted/secret"}, got)

	got, err = FilterLocalFiles(localPath, []string{"**/secret"}, []string{"secret"})
	require.NoError(t, err)
	relativeFiles(localPath, got)
	require.Equal(t, []string{"decrypted/secret", "other/decrypted/secret"}, got)

	got, err = FilterLocalFiles(localPath, nil, []string{"secret"})
	require.NoError(t, err)
	relativeFiles(localPath, got)
	require.Equal(t, []string{"decrypted/secret", "other/decrypted/secret", "src/foo"}, got)

	got, err = FilterLocalFiles(localPath, nil, []string{"**/secret"})
	require.NoError(t, err)
	relativeFiles(localPath, got)
	require.Equal(t, []string{"src/foo"}, got)

	got, err = FilterLocalFiles(localPath+"/secret", nil, nil)
	require.NoError(t, err)
	relativeFiles(localPath, got)
	require.Equal(t, []string{"secret"}, got)
}

func relativeFiles(localPath string, localFiles []string) {
	for i, f := range localFiles {
		localFiles[i], _ = filepath.Rel(localPath, f)
	}
	sort.Strings(localFiles)
}
