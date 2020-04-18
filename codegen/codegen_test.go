package codegen

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/solver"
	"github.com/stretchr/testify/require"
	"github.com/xlab/treeprint"
)

func Expect(t *testing.T, st llb.State) solver.Request {
	def, err := st.Marshal(context.Background(), llb.LinuxAmd64)
	require.NoError(t, err)

	return solver.Single(&solver.Params{
		Def: def,
	})
}

type testCase struct {
	name    string
	targets []string
	input   string
	fn      func(t *testing.T, cg *CodeGen) solver.Request
}

func cleanup(value string) string {
	result := strings.TrimSpace(value)
	result = strings.ReplaceAll(result, strings.Repeat("\t", 3), "")
	result = strings.ReplaceAll(result, "|\n", "| \n")
	return result
}

func TestCodeGen(t *testing.T) {
	for _, tc := range []testCase{{
		"image",
		[]string{"default"},
		`
		fs default() {
			image "alpine"
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Image("alpine"))
		},
	}, {
		"call function",
		[]string{"default"},
		`
		fs default() {
			foo "busybox"
		}

		fs foo(string ref) {
			image ref
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Image("busybox"))
		},
	}, {
		"local",
		[]string{"default"},
		`
		fs default() {
			local "."
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			id, err := cg.LocalID(".")
			require.NoError(t, err)

			return Expect(t, llb.Local(id, llb.SessionID(cg.SessionID()), llb.SharedKeyHint(".")))
		},
	}, {
		"local env",
		[]string{"default"},
		`
		fs default() {
			scratch
			mkfile "home" 0o644 foo
		}

		string foo() {
			localEnv "HOME"
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Scratch().File(llb.Mkfile("home", 0644, []byte(os.Getenv("HOME")))))
		},
	}, {
		"scratch mounts without func lit",
		[]string{"default"},
		`
		fs echo() {
			image "alpine"
			run "touch /out/foo" with option {
				mount scratch "/out" as default
			}
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Image("alpine").Run(
				llb.Shlex("touch /out/foo"),
			).AddMount("/out", llb.Scratch()))
		},
	}, {
		"option builtin without func lit",
		[]string{"default"},
		`
		fs default() {
			image "alpine"
			run "echo unchanged" with readonlyRootfs
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Image("alpine").Run(
				llb.Shlex("echo unchanged"),
				llb.ReadonlyRootFS(),
			).Root())
		},
	}, {
		"empty group",
		[]string{"default"},
		`
		group default() {}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return solver.Sequential()
		},
	}, {
		"sequential group",
		[]string{"default"},
		`
		group default() {
			image "alpine"
			image "busybox"
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return solver.Sequential(
				Expect(t, llb.Image("alpine")),
				Expect(t, llb.Image("busybox")),
			)
		},
	}, {
		"parallel group",
		[]string{"default"},
		`
		group default() {
			parallel group {
				image "alpine"
			} group {
				image "busybox"
			}
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return solver.Parallel(
				Expect(t, llb.Image("alpine")),
				Expect(t, llb.Image("busybox")),
			)
		},
	}, {
		"parallel and sequential groups",
		[]string{"default"},
		`
		group default() {
			image "golang:alpine"
			parallel group {
				image "alpine"
			} group {
				image "busybox"
			}
			image "node:alpine"
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return solver.Sequential(
				Expect(t, llb.Image("golang:alpine")),
				solver.Parallel(
					Expect(t, llb.Image("alpine")),
					Expect(t, llb.Image("busybox")),
				),
				Expect(t, llb.Image("node:alpine")),
			)
		},
	}, {
		"invoking group functions",
		[]string{"default"},
		`
		group default() {
			foo "stable"
		}

		group foo(string ref) {
			image string { format "alpine:%s" ref; }
			image string { format "busybox:%s" ref; }
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return solver.Sequential(
				Expect(t, llb.Image("alpine:stable")),
				Expect(t, llb.Image("busybox:stable")),
			)
		},
	}, {
		"here doc processing",
		[]string{"default"},
		`
		fs default() {
			image "busybox"

			run <<~EOM
			echo
			hi
			EOM

			run <<-EOM
			echo hi
			EOM

			run <<EOM
			echo hi
			EOM
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Image("busybox").Run(
				llb.Shlex("echo hi"),
			).Run(
				llb.Shlex("echo hi"),
			).Run(
				llb.Shlex("\techo hi"),
			).Root())
		},
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cg, err := New()
			require.NoError(t, err)

			mod, err := parser.Parse(strings.NewReader(cleanup(tc.input)))
			require.NoError(t, err)

			err = checker.Check(mod)
			require.NoError(t, err)

			var targets []Target
			for _, target := range tc.targets {
				targets = append(targets, Target{Name: target})
			}

			request, err := cg.Generate(context.Background(), mod, targets)
			require.NoError(t, err)

			testRequest := tc.fn(t, cg)

			expected := treeprint.New()
			testRequest.Tree(expected)
			t.Logf("expected: %s", expected)

			actual := treeprint.New()
			request.Tree(actual)
			t.Logf("actual: %s", actual)

			// Compare trees.
			require.Equal(t, expected.String(), actual.String())
		})
	}
}
