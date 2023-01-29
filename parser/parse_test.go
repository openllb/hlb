package parser

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var def = `
		fs foo() {
			image "alpine" with option {
				resolve
		        }
			run "echo foo" with option {
				readonlyRootfs
				env "key" "value"
				dir "path"
				user "name"
				network unset
				security sandbox
				host "name" "ip"
				ssh with option {
					target "path"
					id "cacheid"
					uid 1000
					gid 1000
					mode 0o700
					optional
				}
				secret "target" with option {
					id "cacheid"
					uid 1000
					mode 0o700
					optional
				}
				mount bar "target" with option {
					readonly
					tmpfs
					source "target"
					cache "cacheid" shared
				}
				forward "tcp://localhost:1234" "/tmp/servicee.sock" with option {
					uid 1000
					gid 1000
					mode 0o666
				}
			}
			env "key" "value"
			dir "path"
			user "name"
			mkdir "path" 0o700 with option {
				createParents
				chown "user:group"
				createdTime "time"
			}
			mkfile "path" 0o700 "content" with option {
				chown "user:group"
				createdTime "time"
			}
			rm "path" with option {
				allowNotFound
				allowWildcard
			}
			copy bar "src" "dst" with option {
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

		fs bar() {
			scratch
			copy fs {
				http "url" with option {
					checksum "digest"
					chmod 0o700
					filename "name"
				}
			} "src" "dst"
			copy fs {
				git "remote" "ref" with option {
					keepGitDir
				}
			} "src" "dst"
		}

		string heredocTest() {
			value <<-EOM
			this
			  should
			dedent
			EOM
			value <<~EOM
			this 
			  should
			fold
			EOM
			value <<EOM
			this
			  is
			literal
			EOM
		}
	`

func TestParse(t *testing.T) {
	t.Parallel()
	file, err := Parse(context.Background(), strings.NewReader(def))
	require.NoError(t, err)
	require.NotNil(t, file)
}
