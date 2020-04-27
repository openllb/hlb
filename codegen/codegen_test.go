package codegen

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/solver/pb"
	"github.com/moby/buildkit/util/entitlements"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/solver"
	"github.com/stretchr/testify/require"
	"github.com/xlab/treeprint"
)

func Expect(t *testing.T, st llb.State, opts ...solver.SolveOption) solver.Request {
	def, err := st.Marshal(context.Background(), llb.LinuxAmd64)
	require.NoError(t, err)

	return solver.Single(&solver.Params{
		Def:       def,
		SolveOpts: opts,
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
	t.Parallel()
	ctx := context.Background()
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
		"basic scratch",
		[]string{"default"},
		`
		fs default() {
			scratch
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Scratch())
		},
	}, {
		"basic http",
		[]string{"default"},
		`
		fs default() {
			http "http://my.test.url"
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.HTTP("http://my.test.url"))
		},
	}, {
		"http with options",
		[]string{"default"},
		`
		fs default() {
			http "http://my.test.url" with option {
				checksum "123"
				chmod 777
				filename "myTest.out"
			}
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.HTTP(
				"http://my.test.url",
				llb.Checksum("123"),
				llb.Chmod(os.FileMode(777)),
				llb.Filename("myTest.out")))
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
			id, err := cg.LocalID(ctx, ".")
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
			parallel fs { image "alpine"; }
			parallel fs { image "busybox"; }
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
			parallel fs {
				image "alpine"
			} fs {
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
			parallel fs { image "golang:alpine"; }
			parallel fs {
				image "alpine"
			} fs {
				image "busybox"
			}
			parallel fs { image "node:alpine"; }
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
			parallel fs {
				image string { format "alpine:%s" ref; }
			}
			parallel fs {
				image string { format "busybox:%s" ref; }
			}
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return solver.Sequential(
				Expect(t, llb.Image("alpine:stable")),
				Expect(t, llb.Image("busybox:stable")),
			)
		},
	}, {
		"parallel coercing fs to group",
		[]string{"default"},
		`
		group default() {
			parallel fs {
				image "alpine"
			} fs {
				scratch
				mkfile "foo" 0o644 "hello world"
				download "."
			}
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return solver.Parallel(
				Expect(t, llb.Image("alpine")),
				solver.Parallel(
					Expect(t, llb.Scratch().File(llb.Mkfile("foo", 0644, []byte("hello world")))),
					Expect(t, llb.Scratch().File(llb.Mkfile("foo", 0644, []byte("hello world"))), solver.WithDownload(".")),
				),
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
	}, {
		"templates",
		[]string{"default"},
		`
		string cmd() {
			template <<-EOM
				echo hi {{.user}}
			EOM with option {
				stringField "user" string {
					localEnv "USER"
				}
			}
		}

		fs default() {
			image "busybox"
			run cmd 
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Image("busybox").Run(
				llb.Shlexf("echo hi %s", os.Getenv("USER")),
			).Root())
		},
	}, {
		"entitlements",
		[]string{"default"},
		`
		fs default() {
			image "busybox"
			run "entitlements" with option {
				network "host"
				security "insecure"
			}
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t,
				llb.Image("busybox").Run(
					llb.Shlex("entitlements"),
					llb.Network(pb.NetMode_HOST),
					llb.Security(pb.SecurityMode_INSECURE),
				).Root(),
				solver.WithEntitlement(entitlements.EntitlementNetworkHost),
				solver.WithEntitlement(entitlements.EntitlementSecurityInsecure),
			)
		},
	}, {
		"mount over readonly",
		[]string{"default"},
		`
		fs default() {
			image "busybox"
			run "find ." with option {
				dir "/foo"
				mount fs {
					local "."
				} "/foo" with readonly
				mount scratch "/foo/bar"
				secret "codegen_test.go" "/foo/secret/codegen_test.go"
			}
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			id, err := cg.LocalID(ctx, ".")
			require.NoError(t, err)
			sid := SecretID("codegen_test.go")

			return Expect(t, llb.Image("busybox").Run(
				llb.Shlex("find ."),
				llb.Dir("/foo"),
				llb.AddMount(
					"/foo",
					llb.Local(
						id,
						llb.SessionID(cg.SessionID()),
						llb.SharedKeyHint("."),
					).File(
						// this Mkdir is made implicitly due to /foo/secret
						// secret over readonly FS
						llb.Mkdir("secret", 0755, llb.WithParents(true)),
					).File(
						// this Mkfile is made implicitly due to /foo/secret
						// secret over readonly FS
						llb.Mkfile("secret/codegen_test.go", 0644, []byte{}),
					).File(
						// this Mkdir is made implicitly due to /foo/bar
						// mount over readonly FS
						llb.Mkdir("bar", 0755, llb.WithParents(true)),
					),
					llb.Readonly,
				),
				llb.AddMount("/foo/bar", llb.Scratch()),
				llb.AddSecret("/foo/secret/codegen_test.go", llb.SecretID(sid)),
			).Root())
		},
	}, {
		"merging user defined option::copy with func lit",
		[]string{"default"},
		`
		fs default() {
			scratch
			copy scratch "/" "/foo" with option {
				createDestPath
				myOpt
			}
		}

		option::copy myOpt() {
			chown "1001:1001"
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Scratch().File(
				llb.Copy(llb.Scratch(), "/", "/foo", &llb.CopyInfo{
					CreateDestPath: true,
					ChownOpt: &llb.ChownOpt{
						User:  &llb.UserOpt{UID: 1001},
						Group: &llb.UserOpt{UID: 1001},
					},
				}),
			))
		},
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
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
