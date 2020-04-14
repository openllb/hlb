package codegen

import (
	"context"
	"strings"
	"testing"

	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/solver"
	"github.com/stretchr/testify/require"
	"github.com/xlab/treeprint"
)

func Expect(st llb.State) solver.Request {
	def, err := st.Marshal(context.Background(), llb.LinuxAmd64)
	if err != nil {
		panic(err)
	}

	return solver.Single(&solver.Params{
		Def: def,
	})
}

type testCase struct {
	name    string
	targets []string
	input   string
	fn      func(cg *CodeGen) (solver.Request, error)
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
		func(cg *CodeGen) (solver.Request, error) {
			return solver.Parallel(
				Expect(llb.Image("alpine")),
			), nil
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
		func(cg *CodeGen) (solver.Request, error) {
			return solver.Parallel(
				Expect(llb.Image("busybox")),
			), nil
		},
	}, {
		"local",
		[]string{"default"},
		`
		fs default() {
			local "."
		}
		`,
		func(cg *CodeGen) (solver.Request, error) {
			id, err := cg.LocalID(".")
			if err != nil {
				return nil, err
			}

			return solver.Parallel(
				Expect(llb.Local(id, llb.SessionID(cg.SessionID()))),
			), nil
		},
		// }, {
		// 	"sequential group",
		// 	[]string{"default"},
		// 	`
		// 	group default() {
		// 		image "alpine"
		// 		image "busybox"
		// 	}
		// 	`,
		// 	func(cg *CodeGen) (solver.Request, error) {
		// 		return solver.Sequential(
		// 			Expect(llb.Image("alpine")),
		// 			Expect(llb.Image("busybox")),
		// 		), nil
		// 	},
		// }, {
		// 	"parallel group",
		// 	[]string{"default"},
		// 	`
		// 	group default() {
		// 		parallel group {
		// 			image "alpine"
		// 		} group {
		// 			image "busybox"
		// 		}
		// 	}
		// 	`,
		// 	func(cg *CodeGen) (solver.Request, error) {
		// 		return solver.Parallel(
		// 			Expect(llb.Image("alpine")),
		// 			Expect(llb.Image("busybox")),
		// 		), nil
		// 	},
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

			testRequest, err := tc.fn(cg)
			require.NoError(t, err)

			expected := treeprint.New()
			testRequest.Tree(expected)

			actual := treeprint.New()
			request.Tree(actual)
			t.Log(actual.String())

			// Compare trees.
			require.Equal(t, expected.String(), actual.String())
		})
	}
}
