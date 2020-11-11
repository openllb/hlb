package llbutil

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/moby/buildkit/client/llb"
)

// MountRunOption gives access to capture custom MountOptions so we
// can easily capture if the mount is to be readonly
type MountRunOption struct {
	Source llb.State
	Target string
	Opts   []interface{}
}

type ReadonlyMount struct{}

func (m *MountRunOption) SetRunOption(es *llb.ExecInfo) {
	opts := []llb.MountOption{}
	for _, opt := range m.Opts {
		switch o := opt.(type) {
		case *ReadonlyMount:
			opts = append(opts, llb.Readonly)
		case llb.MountOption:
			opts = append(opts, o)
		}
	}
	llb.AddMount(m.Target, m.Source, opts...).SetRunOption(es)
}

func (m *MountRunOption) IsReadonly() bool {
	for _, opt := range m.Opts {
		if _, ok := opt.(*ReadonlyMount); ok {
			return true
		}
	}
	return false
}

// ShimReadonlyMountpoints will modify the source for readonly mounts so
// subsequent mounts that mount onto the readonly-mounts will have the
// mountpoint present.
//
// For example if we have this code:
//
// ```hlb
// run "make" with option {
// 	dir "/src"
//	mount fs {
//		local "."
//	} "/src" with readonly
//		mount scratch "/src/output" as buildOutput
//		# ^^^^^ FAIL cannot create `output` directory for mount on readonly fs
//		secret "./secret/foo.pem" "/src/secret/foo.pem"
//		# ^^^^^ FAIL cannot create `./secret/foo.pm` for secret on readonly fs
//	}
// }
// ```
//
// When containerd tries to mount /src/output on top of the /src mountpoint it
// will fail because /src is mounted as readonly.  The work around for this is
// to inline create the mountpoints so that they pre-exist and containerd will
// not have to create them.
//
// It can be done with HLB like:
//
// ```hlb
// run "make" with option {
// 	dir "/src"
//	mount fs {
//		local "."
//		mkdir "output" 0o755 # <-- this is added to ensure mountpoint exists
//		mkdir "secret" 0o755            # <-- added so the secret can be mounted
//		mkfile "secret/foo.pm" 0o644 "" # <-- added so the secret can be mounted
//	} "/src" with readonly
//		mount scratch "/src/output" as buildOutput
//	}
// }
// ```
//
// So this function is effectively automatically adding the `mkdir` and `mkfile`
// instructions when it detects that a mountpoint is required to be on a
// readonly fs.
func ShimReadonlyMountpoints(opts []llb.RunOption) error {
	// Short-circuit if we don't have any readonly mounts.
	haveReadonly := false
	for _, opt := range opts {
		if mnt, ok := opt.(*MountRunOption); ok {
			haveReadonly = mnt.IsReadonly()
			if haveReadonly {
				break
			}
		}
	}
	if !haveReadonly {
		return nil
	}

	// Collecting run options to look for targets (secrets, mounts) so we can
	// determine if there are overlapping mounts with readonly attributes.
	mountDetails := make([]struct {
		Target string
		Mount  *MountRunOption
	}, len(opts))

	for i, opt := range opts {
		switch runOpt := opt.(type) {
		case *MountRunOption:
			mountDetails[i].Target = runOpt.Target
			mountDetails[i].Mount = runOpt
		case llb.RunOption:
			ei := llb.ExecInfo{}
			runOpt.SetRunOption(&ei)
			if len(ei.Secrets) > 0 {
				// We only processed one option, so can have at most one secret.
				mountDetails[i].Target = ei.Secrets[0].Target
				continue
			}
		}
	}

	// madeDirs will keep track of directories we have had to create
	// so we don't duplicate instructions.
	madeDirs := map[string]struct{}{}

	// If we have readonly mounts and then secrets or other mounts on top of the
	// readonly mounts then we have to run a mkdir or mkfile on the mount first
	// before it become readonly.

	// Now walk the mountDetails backwards and look for common target paths
	// in prior mounts (mount ordering is significant).
	for i := len(mountDetails) - 1; i >= 0; i-- {
		src := mountDetails[i]
		if src.Target == "" {
			// Not a target option, like `dir "foo"`, so just skip.
			continue
		}
		for j := i - 1; j >= 0; j-- {
			dest := mountDetails[j]
			if !strings.HasPrefix(src.Target, dest.Target) {
				// Paths not common, skip.
				continue
			}
			if dest.Mount == nil {
				// Dest is not a mount, so skip.
				continue
			}
			if !dest.Mount.IsReadonly() {
				// Not mounting into readonly fs, so we are good with this mount.
				break
			}

			// We need to rewrite the mount at opts[j] so that that we mkdir and/or
			// mkfile.
			st := dest.Mount.Source
			if src.Mount != nil {
				// This is a mount, so we need to ensure the mount point
				// directory has been created.
				if _, ok := madeDirs[src.Target]; ok {
					// Already created the dir.
					break
				}

				// Update local cache so we don't make this dir again.
				madeDirs[dest.Target] = struct{}{}

				relativeDir, err := filepath.Rel(dest.Target, src.Target)
				if err != nil {
					return err
				}
				st = st.File(
					llb.Mkdir(relativeDir, os.FileMode(0755), llb.WithParents(true)),
				)
			} else {
				// Not a mount, so must be a `secret` which will be a path to a file, we
				// will need to make the directory for the secret as well as an empty file
				// to be mounted over.
				dir := filepath.Dir(src.Target)
				relativeDir := strings.TrimPrefix(dir, dest.Target)

				if _, ok := madeDirs[dir]; !ok {
					// Update local cache so we don't make this dir again.
					madeDirs[dir] = struct{}{}

					st = st.File(
						llb.Mkdir(relativeDir, os.FileMode(0755), llb.WithParents(true)),
					)
				}

				relativeFile, err := filepath.Rel(dest.Target, src.Target)
				if err != nil {
					return err
				}

				st = st.File(
					llb.Mkfile(relativeFile, os.FileMode(0644), []byte{}),
				)
			}

			// Reset the mount option to include our state with mkdir/mkfile actions.
			opts[j] = &MountRunOption{
				Target: dest.Target,
				Source: st,
				Opts:   dest.Mount.Opts,
			}

			// Save the state for later in case we need to add more mkdir/mkfile actions.
			mountDetails[j].Mount.Source = st
			break
		}
	}

	return nil
}
