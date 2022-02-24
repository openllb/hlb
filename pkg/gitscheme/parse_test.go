package gitscheme

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	type testCase struct {
		name                                          string
		uri                                           string
		scheme, user, host, gitPath, branch, filename string
	}

	for _, tc := range []testCase{{
		"short",
		"git://git.personal.com",
		"git", "", "git.personal.com", "", "", "",
	}, {
		"long",
		"git://git.personal.com/path/to/deep/repo",
		"git", "", "git.personal.com", "/path/to/deep/repo", "", "",
	}, {
		"github host",
		"git://github.com/openllb/hlb",
		"git", "", "github.com", "/openllb/hlb", "", "",
	}, {
		"branch",
		"git://github.com/openllb/hlb@main",
		"git", "", "github.com", "/openllb/hlb", "main", "",
	}, {
		"branch with @",
		"git://github.com/openllb/hlb@a@b",
		"git", "", "github.com", "/openllb/hlb", "a@b", "",
	}, {
		"file",
		"git://github.com/openllb/hlb:file",
		"git", "", "github.com", "/openllb/hlb", "", "file",
	}, {
		"file in subdir",
		"git://github.com/openllb/hlb:/sub/dir/file",
		"git", "", "github.com", "/openllb/hlb", "", "/sub/dir/file",
	}, {
		"branch and file",
		"git://github.com/openllb/hlb@develop:/sub/dir/file",
		"git", "", "github.com", "/openllb/hlb", "develop", "/sub/dir/file",
	}, {
		"repo with colons",
		"git://git.personal.com:1234/hello:world:/file",
		"git", "", "git.personal.com:1234", "/hello:world", "", "/file",
	}, {
		"git+ssh",
		"git+ssh://git@git.personal.com:1234/~user/repo.git@main:/file",
		"git+ssh", "git", "git.personal.com:1234", "/~user/repo.git", "main", "/file",
	}, {
		"git+https",
		"git+https://github.com/openllb/hlb.git@main:/file",
		"git+https", "", "github.com", "/openllb/hlb.git", "main", "/file",
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			uri, err := Parse(tc.uri)
			require.NoError(t, err)
			require.NotNil(t, uri)
			require.Equal(t, tc.scheme, uri.Scheme)
			require.Equal(t, tc.user, uri.User)
			require.Equal(t, tc.host, uri.Host)
			require.Equal(t, tc.gitPath, uri.Path)
			require.Equal(t, tc.branch, uri.Branch)
			require.Equal(t, tc.filename, uri.Filename)
		})
	}

}
