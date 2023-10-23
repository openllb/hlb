package solver

import (
	"context"
	"io"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/creack/pty"
	"github.com/docker/buildx/util/progress"
	"github.com/stretchr/testify/require"
)

func TestProgress(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		fn   func(p Progress) error
	}

	ctx := context.Background()
	for _, tc := range []testCase{{
		"empty opts", nil,
	}, {
		"output empty sync",
		func(p Progress) error {
			// Can sync from beginning.
			err := p.Sync()
			if err != nil {
				return err
			}

			// Can sync after sync.
			return p.Sync()
		},
	}, {
		"output sync after write",
		func(p Progress) error {
			pw := p.MultiWriter().WithPrefix("", false)
			if err := progress.Wrap("test", pw.Write, func(l progress.SubLogger) error {
				return ProgressFromReader(l, io.NopCloser(strings.NewReader("")))
			}); err != nil {
				return err
			}

			// Can sync after write.
			return p.Sync()
		},
	}} {
		for _, mode := range []string{"tty", "plain"} {
			tc, mode := tc, mode
			t.Run(tc.name+" "+mode, func(t *testing.T) {
				ptm, pts, err := pty.Open()
				require.NoError(t, err)

				var opts []ProgressOption
				switch mode {
				case "tty":
					opts = append(opts, WithLogOutputTTY(pts))
				case "plain":
					opts = append(opts, WithLogOutputPlain(pts))
				}

				p, err := NewProgress(ctx, opts...)
				require.NoError(t, err)

				if tc.fn != nil {
					err = tc.fn(p)
					require.NoError(t, err)
				}

				err = p.Wait()
				require.NoError(t, err)

				err = pts.Close()
				require.NoError(t, err)

				data, _ := ioutil.ReadAll(ptm)
				t.Log("\n" + string(data))

				err = ptm.Close()
				require.NoError(t, err)
			})
		}
	}
}
