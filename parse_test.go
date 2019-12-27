package hlb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	fullHLB = `
		state foo {
			image "alpine" with option {
				resolve
		        }
			exec "echo foo" with option {
				readonlyRootfs
				env "key" "value"
				dir "path"
				user "name"
				network unset
				security sandbox
				host "name" "ip"
				ssh with option {
					mountpoint "path"
					id "cacheid"
					uid 1000
					gid 1000
					mode 0700
					optional
				}
				secret "mountpoint" with option {
					id "cacheid"
					uid 1000
					mode 0700
					optional
				}
				mount bar "mountpoint" with option {
					readonly
					tmpfs
					source "target"
					cache "cacheid" shared
				}
			}
			env "key" "value"
			dir "path"
			user "name"
			mkdir "path" 0700 with option {
				createParents
				chown "user:group"
				createdTime "time"
			}
			mkfile "path" 0700 "content" with option {
				chown "user:group"
				createdTime "time"
			}
			rm "path" with option {
				allowNotFound
				allowWildcard
			}
			copy { from bar } "src" "dst" with option {
				followSymlinks
				contentsOnly
				unpack
				createDestPath
				allowWildcard
				allowEmptyWildcard
				chown "user:group"
				createdTime "time"
			}
		}

		state bar {
			scratch
			copy {
				http "url" with option {
					checksum "digest"
					chmod 0700
					filename "name"
				}
			} "src" "dst"
			copy {
				git "remote" "ref" with option {
					keepGitDir
				}
			} "src" "dst"
		}
	`
)

func TestParse(t *testing.T) {
	ast, err := Parse(strings.NewReader(fullHLB))
	require.NoError(t, err)

	require.Equal(t, 2, len(ast.Entries))
	require.NotNil(t, ast.Entries[0].State)
	require.NotNil(t, ast.Entries[1].State)

	// state foo
	foo := ast.Entries[0].State
	require.Equal(t, "foo", foo.Name)
	require.NotNil(t, foo.Body)
	require.Equal(t, 8, len(foo.Body.Ops))
	require.NotNil(t, foo.Body.Source)

	// state foo image
	image := foo.Body.Source.Image
	require.NotNil(t, image)
	require.NotNil(t, "alpine", image.Ref)
	require.NotNil(t, image.Option)
	require.Equal(t, 1, len(image.Option.ImageFields))
	require.NotNil(t, image.Option.ImageFields[0].Resolve)

	// state foo exec
	exec := foo.Body.Ops[0].Exec
	require.NotNil(t, exec)
	require.Equal(t, "echo foo", exec.Shlex)
	require.NotNil(t, exec.Option)
	require.Equal(t, 10, len(exec.Option.ExecFields))

	// state bar
	bar := ast.Entries[1].State
	require.Equal(t, "bar", bar.Name)
	require.NotNil(t, bar.Body)
	require.Equal(t, 2, len(bar.Body.Ops))
}
