package codegen

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lithammer/dedent"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/solver/pb"
	"github.com/moby/buildkit/util/entitlements"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
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

func (cg *CodeGen) Local(t *testing.T, path string, opts ...llb.LocalOption) llb.State {
	id, err := cg.LocalID(context.Background(), ".", opts...)
	require.NoError(t, err)

	opts = append([]llb.LocalOption{
		llb.SessionID(cg.SessionID()),
		llb.SharedKeyHint(path),
	}, opts...)

	return llb.Local(id, opts...)
}

type testCase struct {
	name    string
	targets []string
	input   string
	fn      func(t *testing.T, cg *CodeGen) solver.Request
}

func cleanup(value string) string {
	return dedent.Dedent(value)
}

func TestCodeGen(t *testing.T) {
	t.Parallel()
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
				chmod 0x777
				filename "myTest.out"
			}
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.HTTP(
				"http://my.test.url",
				llb.Checksum("123"),
				llb.Chmod(os.FileMode(0x777)),
				llb.Filename("myTest.out")))
		},
	}, {
		"basic git",
		[]string{"default"},
		`
		fs default() {
			git "https://github.com/openllb/hlb.git" "master"
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Git("https://github.com/openllb/hlb.git", "master"))
		},
	}, {
		"git with options",
		[]string{"default"},
		`
		fs default() {
			git "https://github.com/openllb/hlb.git" "master" with option {
				keepGitDir
			}
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Git(
				"https://github.com/openllb/hlb.git",
				"master",
				llb.KeepGitDir()))
		},
	}, {
		"basic mkdir",
		[]string{"default"},
		`
		fs default() {
			scratch
			mkdir "testDir" 0x777
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Scratch().File(llb.Mkdir("testDir", os.FileMode(0x777))))
		},
	}, {
		"mkdir with options",
		[]string{"default"},
		`
		fs default() {
			scratch
			mkdir "testDir" 0x777 with option {
				createParents
				chown "testUser"
				createdTime "2020-04-27T15:04:05Z"
			}
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			createdTime, err := time.Parse(time.RFC3339, "2020-04-27T15:04:05Z")
			require.NoError(t, err)

			return Expect(t, llb.Scratch().File(llb.Mkdir(
				"testDir",
				os.FileMode(0x777),
				llb.WithParents(true),
				llb.WithUser("testUser"),
				llb.WithCreatedTime(createdTime))))
		},
	}, {
		"basic env",
		[]string{"default"},
		`
		fs default() {
			scratch
			env "TEST_VAR" "test value"
			run "echo Hello" with shlex
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Scratch().AddEnv("TEST_VAR", "test value").Run(llb.Shlex("echo Hello")).Root())
		},
	}, {
		"basic dir",
		[]string{"default"},
		`
		fs default() {
			scratch
			dir "testDir"
			run "echo Hello" with shlex
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Scratch().Dir("testDir").Run(llb.Shlex("echo Hello")).Root())
		},
	}, {
		"basic user",
		[]string{"default"},
		`
		fs default() {
			scratch
			user "testUser"
			run "echo Hello" with shlex
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Scratch().User("testUser").Run(llb.Shlex("echo Hello")).Root())
		},
	}, {
		"basic mkfile",
		[]string{"default"},
		`
		fs default() {
			scratch
			mkfile "testFile" 0x777 "Hello"
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Scratch().File(llb.Mkfile("testFile", os.FileMode(0x777), []byte("Hello"))))
		},
	}, {
		"mkfile with options",
		[]string{"default"},
		`
		fs default() {
			scratch
			mkfile "testFile" 0x777 "Hello" with option {
				chown "testUser"
				createdTime "2020-04-27T15:04:05Z"
			}
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			createdTime, err := time.Parse(time.RFC3339, "2020-04-27T15:04:05Z")
			require.NoError(t, err)

			return Expect(t, llb.Scratch().File(llb.Mkfile(
				"testFile",
				os.FileMode(0x777),
				[]byte("Hello"),
				llb.WithUser("testUser"),
				llb.WithCreatedTime(createdTime))))
		},
	}, {
		"basic rm",
		[]string{"default"},
		`
		fs default() {
			scratch
			rm "testFile"
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Scratch().File(llb.Rm("testFile")))
		},
	}, {
		"rm with options",
		[]string{"default"},
		`
		fs default() {
			scratch
			rm "testFile" with option {
				allowNotFound
				allowWildcard
			}
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Scratch().File(llb.Rm(
				"testFile",
				llb.WithAllowNotFound(true),
				llb.WithAllowWildcard(true))))
		},
	}, {
		"basic copy",
		[]string{"default"},
		`
		fs default() {
			scratch as this
			copy this "testSource" "testDest"
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			scratch := llb.Scratch()
			return Expect(t, scratch.File(llb.Copy(scratch, "testSource", "testDest")))
		},
	}, {
		"copy with options",
		[]string{"default"},
		`
		fs default() {
			scratch as this
			copy this "testSource" "testDest" with option {
				followSymlinks
				contentsOnly
				unpack
				createDestPath
				allowWildcard
				allowEmptyWildcard
				chown "testUser"
				chmod 0x777
				createdTime "2020-04-27T15:04:05Z"
			}
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			createdTime, err := time.Parse(time.RFC3339, "2020-04-27T15:04:05Z")
			require.NoError(t, err)

			fileMode := os.FileMode(0x777)
			copyInfo := llb.CopyInfo{
				Mode:                &fileMode,
				FollowSymlinks:      true,
				CopyDirContentsOnly: true,
				AttemptUnpack:       true,
				CreateDestPath:      true,
				AllowWildcard:       true,
				AllowEmptyWildcard:  true,
			}

			scratch := llb.Scratch()
			return Expect(t, scratch.File(llb.Copy(
				scratch,
				"testSource",
				"testDest",
				&copyInfo,
				llb.WithUser("testUser"),
				llb.WithCreatedTime(createdTime),
			)))
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
			return Expect(t, cg.Local(t, "."))
		},
	}, {
		"local file",
		[]string{"default"},
		`
		fs default() {
			local "codegen_test.go"
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, cg.Local(t, ".",
				llb.IncludePatterns([]string{"codegen_test.go"}),
			))
		},
	}, {
		"local file with patterns",
		[]string{"default"},
		`
		fs default() {
			local "codegen_test.go" with option {
				includePatterns "ignored"
				excludePatterns "ignored"
			}
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, cg.Local(t, ".",
				llb.IncludePatterns([]string{"codegen_test.go"}),
			))
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
				shlex
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
				llb.Args([]string{"/bin/sh", "-c", "echo hi"}),
			).Run(
				llb.Args([]string{"/bin/sh", "-c", "echo hi"}),
			).Run(
				llb.Args([]string{"/bin/sh", "-c", "\techo hi"}),
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
			run cmd with shlex
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
					llb.Args([]string{"/bin/sh", "-c", "entitlements"}),
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
				shlex
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
			sid := SecretID("codegen_test.go")
			return Expect(t, llb.Image("busybox").Run(
				llb.Shlex("find ."),
				llb.Dir("/foo"),
				llb.AddMount(
					"/foo",
					cg.Local(
						t,
						".",
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
	}, {
		"localRun",
		[]string{"default"},
		`
		fs default() {
			mkfile "./just-stdout" 0o644 string {
				localRun "echo stdout; echo stderr >&2"
			}
			mkfile "./just-stderr" 0o644 string {
				localRun "echo stdout; echo stderr >&2" with onlyStderr
			}
			mkfile "./stdio" 0o644 string {
				localRun "echo stdout; echo stderr >&2" with includeStderr
			}
			mkfile "./goterror" 0o644 string {
				localRun "echo stdout; exit 1" with ignoreError
			}
			# this will write evaluate the $HOME env var because run as ["/bin/sh", "-c", "echo $HOME"]
			mkfile "./noshlex" 0o644 string {
				localRun "echo $HOME"
			}
			# this will write the literal string "$HOME" because run as ["/bin/echo", "$HOME"]
			mkfile "./shlex" 0o644 string {
				localRun "echo $HOME" with shlex
			}
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return Expect(t, llb.Scratch().File(
				llb.Mkfile("just-stdout", os.FileMode(0644), []byte("stdout\n")),
			).File(
				llb.Mkfile("just-stderr", os.FileMode(0644), []byte("stderr\n")),
			).File(
				llb.Mkfile("stdio", os.FileMode(0644), []byte("stdout\nstderr\n")),
			).File(
				llb.Mkfile("goterror", os.FileMode(0644), []byte("stdout\n")),
			).File(
				llb.Mkfile("noshlex", os.FileMode(0644), []byte(fmt.Sprintf("%s\n", os.Getenv("HOME")))),
			).File(
				llb.Mkfile("shlex", os.FileMode(0644), []byte("$HOME\n")),
			))
		},
	}, {
		"dockerfile meta",
		[]string{"default"},
		`
		fs default() {
			image "busybox"
			env "myenv1" "value1"
			env "myenv2" "value2"
			env "myenv1" "value3"
			dir "myworkdir"
			entrypoint "my" "entrypoint"
			cmd "my" "cmd"
			label "mylabel1" "value1"
			label "mylabel2" "value2"
			label "mylabel1" "value3"
			expose "8080/tcp" "9001/udp"
			volumes "/var/log" "/var/db"
			stopSignal "SIGKILL"
			dockerPush "myimage"
		}
		`,
		func(t *testing.T, cg *CodeGen) solver.Request {
			return solver.Parallel(
				Expect(t, llb.Image("busybox")),
				Expect(t, llb.Image("busybox"),
					solver.WithPushImage("myimage"),
					solver.WithImageSpec(&specs.Image{
						Config: specs.ImageConfig{
							Env:        []string{"myenv2=value2", "myenv1=value3"},
							WorkingDir: "/myworkdir",
							Entrypoint: []string{"my", "entrypoint"},
							Cmd:        []string{"my", "cmd"},
							Labels: map[string]string{
								"mylabel1": "value3",
								"mylabel2": "value2",
							},
							ExposedPorts: map[string]struct{}{
								"8080/tcp": {},
								"9001/udp": {},
							},
							Volumes: map[string]struct{}{
								"/var/log": {},
								"/var/db":  {},
							},
							StopSignal: "SIGKILL",
						},
					}),
				),
			)
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
