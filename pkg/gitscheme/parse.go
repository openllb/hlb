package gitscheme

import (
	"net/url"
	"strings"
)

type URI struct {
	Scheme   string
	User     string
	Host     string
	Path     string
	Branch   string
	Filename string
}

// Parse parses a Git URI scheme that supports referencing a file in a git
// repository in a specific branch or commit.
//
// The branch and filepath are optional.
// If branch is omitted, based on the host, the default branch for the
// repository is retrieved.
func Parse(uri string) (*URI, error) {
	// Example uri: git://github.com/openllb/hlb@main:build.hlb
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	// The ":" character was chosen to split git repository from the reference
	// to a file in the repository. Since Windows doesn't support ":" in directory
	// names, it seems to be a safe choice for git repositories. This is also
	// consistent with the SSH URI scheme.
	var gitPath, filename string
	parts := strings.Split(u.Path, ":")
	if len(parts) > 1 {
		gitPath, filename = strings.Join(parts[:len(parts)-1], ":"), parts[len(parts)-1]
	} else {
		gitPath = parts[0]
	}

	// The "@" character was chosen to specify the branch or commit of a git
	// repository. Since "@" is not a valid character for GitHub repository names,
	// it seems to be a safe choice. This is also consistent with `go get`
	// starting with Go 1.11 when using Go modules.
	var branch string
	parts = strings.SplitN(gitPath, "@", 2)
	if len(parts) > 1 {
		// "@" is a valid character in git branches, so we must join the rest.
		gitPath, branch = parts[0], parts[1]
	}

	return &URI{
		Scheme:   u.Scheme,
		User:     u.User.String(),
		Host:     u.Host,
		Path:     gitPath,
		Branch:   branch,
		Filename: filename,
	}, nil
}
