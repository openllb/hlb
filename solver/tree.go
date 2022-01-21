package solver

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/kballard/go-shellquote"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/solver/pb"
	"github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
	"github.com/xlab/treeprint"
)

func TreeFromDef(tree treeprint.Tree, def *llb.Definition, opts []SolveOption) error {
	var info SolveInfo
	for _, opt := range opts {
		err := opt(&info)
		if err != nil {
			return err
		}
	}

	ops := make(map[digest.Digest]*pb.Op)

	var dgst digest.Digest
	for _, dt := range def.Def {
		var op pb.Op
		if err := (&op).Unmarshal(dt); err != nil {
			return err
		}
		dgst = digest.FromBytes(dt)
		ops[dgst] = &op
	}

	if dgst == "" {
		return nil
	}

	terminal := ops[dgst]
	child := op{dgst: terminal.Inputs[0].Digest, ops: ops, meta: def.Metadata, info: info}
	return child.Tree(tree)
}

type op struct {
	dgst digest.Digest
	ops  map[digest.Digest]*pb.Op
	meta map[digest.Digest]pb.OpMetadata
	info SolveInfo
}

func (o op) Tree(tree treeprint.Tree) error {
	pbOp := o.ops[o.dgst]

	var branch treeprint.Tree

	reportedInputs := map[digest.Digest]struct{}{}

	switch v := pbOp.Op.(type) {
	case *pb.Op_Source:
		var keys []string
		for key := range v.Source.Attrs {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		branch = tree.AddMetaBranch("source", v.Source.Identifier)
		for _, key := range keys {
			branch.AddMetaNode(key, v.Source.Attrs[key])
		}
	case *pb.Op_Exec:
		meta := v.Exec.Meta
		cmd := shellquote.Join(meta.Args...)
		if o.meta[o.dgst].IgnoreCache {
			cmd += " [ignoreCache]"
		}
		branch = tree.AddMetaBranch("exec", cmd)
		if len(meta.Env) > 0 {
			for _, env := range meta.Env {
				branch.AddMetaNode("env", env)
			}
		}

		if meta.Cwd != "" {
			branch.AddMetaNode("cwd", meta.Cwd)
		}
		if meta.User != "" {
			branch.AddMetaNode("user", meta.User)
		}

		for _, input := range pbOp.Inputs {
			reportedInputs[input.Digest] = struct{}{}
		}

		for _, mnt := range v.Exec.Mounts {
			opts := fmt.Sprintf("type=%s", mnt.MountType)
			if mnt.Readonly {
				opts += ",ro"
			}
			if mnt.CacheOpt != nil {
				opts += fmt.Sprintf(",cache-id=%s", mnt.CacheOpt.ID)
				opts += fmt.Sprintf(",sharing=%s", mnt.CacheOpt.Sharing)
			}
			if mnt.SecretOpt != nil {
				opts += fmt.Sprintf(",secret=%s", mnt.SecretOpt.ID)
			}
			if mnt.SSHOpt != nil {
				opts += fmt.Sprintf(",ssh=%s", mnt.SSHOpt.ID)
			}

			mountBranch := branch.AddMetaBranch("mount", fmt.Sprintf("%s [%s]", mnt.Dest, opts))
			if mnt.Input >= 0 && int(mnt.Input) < len(pbOp.Inputs) {
				child := op{dgst: pbOp.Inputs[mnt.Input].Digest, ops: o.ops, meta: o.meta}
				err := child.Tree(mountBranch)
				if err != nil {
					return err
				}
			} else {
				mountBranch.AddNode("scratch")
			}
		}

	case *pb.Op_File:
		branch = tree.AddMetaBranch("file", v.File)
	case *pb.Op_Build:
		branch = tree.AddMetaBranch("build", v.Build)
	case *pb.Op_Merge:
		branch = tree.AddMetaBranch("merge", v.Merge)
	case *pb.Op_Diff:
		branch = tree.AddMetaBranch("diff", v.Diff)
	default:
		return errors.Errorf("unrecognized op %T", pbOp.Op)
	}

	var solve treeprint.Tree
	initSolve := func() {
		if solve == nil {
			solve = branch.AddBranch("solve options")
		}
	}
	if o.info.OutputDockerRef != "" {
		initSolve()
		solve.AddMetaNode("dockerRef", o.info.OutputDockerRef)
	}
	if o.info.OutputPushImage != "" {
		initSolve()
		solve.AddMetaNode("pushImage", o.info.OutputPushImage)
	}
	if o.info.OutputLocal != "" {
		initSolve()
		solve.AddMetaNode("download", o.info.OutputLocal)
	}
	if o.info.OutputLocalTarball {
		initSolve()
		solve.AddNode("downloadTarball")
	}
	if o.info.OutputLocalOCITarball {
		initSolve()
		solve.AddNode("downloadOCITarball")
	}
	if o.info.ImageSpec != nil {
		initSolve()
		dt, err := json.Marshal(o.info.ImageSpec)
		if err != nil {
			return err
		}
		solve.AddMetaNode("imageSpec", string(dt))
	}
	if len(o.info.Entitlements) > 0 {
		initSolve()
		ent := solve.AddBranch("entitlements")
		for _, entitlements := range o.info.Entitlements {
			ent.AddNode(string(entitlements))
		}
	}

	if pbOp.Platform != nil {
		branch.AddMetaNode("platform", fmt.Sprintf("%s,%s", pbOp.Platform.OS, pbOp.Platform.Architecture))
	}

	if pbOp.Constraints != nil && len(pbOp.Constraints.Filter) > 0 {
		constraints := branch.AddBranch("constraints")
		for _, filter := range pbOp.Constraints.Filter {
			constraints.AddNode(filter)
		}
	}

	for _, input := range pbOp.Inputs {
		if _, ok := reportedInputs[input.Digest]; ok {
			continue
		}
		child := op{dgst: input.Digest, ops: o.ops, meta: o.meta}
		err := child.Tree(branch)
		if err != nil {
			return err
		}
	}

	return nil
}
