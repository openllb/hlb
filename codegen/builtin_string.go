package codegen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"text/template"

	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/docker/distribution/reference"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb/sourceresolver"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/local"
	"github.com/openllb/hlb/pkg/imageutil"
)

type Format struct{}

func (f Format) Call(ctx context.Context, cln *client.Client, val Value, opts Option, formatStr string, values ...string) (Value, error) {
	var a []interface{}
	for _, value := range values {
		a = append(a, value)
	}
	return NewValue(ctx, fmt.Sprintf(formatStr, a...))
}

type Template struct{}

func (t Template) Call(ctx context.Context, cln *client.Client, val Value, opts Option, text string) (Value, error) {
	tmpl, err := template.New("").Parse(text)
	if err != nil {
		return nil, err
	}

	data := map[string]interface{}{}
	for _, opt := range opts {
		switch o := opt.(type) {
		case *TemplateField:
			data[o.Name] = o.Value
		}
	}

	buf := bytes.NewBufferString("")
	err = tmpl.Execute(buf, data)
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, buf.String())
}

type LocalArch struct{}

func (la LocalArch) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	return NewValue(ctx, local.Arch(ctx))
}

type LocalCwd struct{}

func (lc LocalCwd) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	cwd, err := local.Cwd(ctx)
	if err != nil {
		return nil, err
	}
	return NewValue(ctx, cwd)
}

type LocalOS struct{}

func (lo LocalOS) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	return NewValue(ctx, local.Os(ctx))
}

type LocalEnv struct{}

func (le LocalEnv) Call(ctx context.Context, cln *client.Client, val Value, opts Option, key string) (Value, error) {
	return NewValue(ctx, local.Env(ctx, key))
}

type LocalRun struct{}

func (lr LocalRun) Call(ctx context.Context, cln *client.Client, val Value, opts Option, args ...string) (Value, error) {
	var (
		localRunOpts = &LocalRunOption{}
		shlex        = false
	)
	for _, opt := range opts {
		switch o := opt.(type) {
		case func(*LocalRunOption):
			o(localRunOpts)
		case *Shlex:
			shlex = true
		}
	}

	runArgs, err := ShlexArgs(args, shlex)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, runArgs[0], runArgs[1:]...)
	cmd.Env = local.Environ(ctx)
	cmd.Dir = ModuleDir(ctx)

	var buf strings.Builder
	if localRunOpts.OnlyStderr {
		cmd.Stderr = &buf
	} else {
		cmd.Stdout = &buf
	}
	if localRunOpts.IncludeStderr {
		cmd.Stderr = &buf
	}

	err = cmd.Run()
	if err != nil && !localRunOpts.IgnoreError {
		return nil, err
	}

	return NewValue(ctx, strings.TrimRight(buf.String(), "\n"))
}

type Manifest struct{}

func (m Manifest) Call(ctx context.Context, cln *client.Client, val Value, opts Option, ref string) (Value, error) {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return nil, errdefs.WithInvalidImageRef(err, Arg(ctx, 0), ref)
	}
	ref = reference.TagNameOnly(named).String()

	var (
		resolver = imageutil.NewBufferedImageResolver()
		matcher  = resolver.MatchDefaultPlatform()
	)
	var platform *specs.Platform
	for _, opt := range opts {
		if p, ok := opt.(*specs.Platform); ok {
			matcher = platforms.Only(*p)
			platform = p
		}
	}

	dgst, config, err := resolver.ResolveImageConfig(ctx, ref, sourceresolver.Opt{Platform: platform})
	if err != nil {
		return nil, err
	}
	if dgst == "" {
		return nil, fmt.Errorf("no digest available for ref %q", ref)
	}

	desc, err := resolver.DigestDescriptor(ctx, dgst)
	if err != nil {
		return nil, err
	}

	switch Binding(ctx).Binds() {
	case "digest":
		return NewValue(ctx, dgst.String())
	case "config":
		return NewValue(ctx, string(config))
	case "index":
		switch desc.MediaType {
		case images.MediaTypeDockerSchema2ManifestList,
			specs.MediaTypeImageIndex:
			ra, err := resolver.ReaderAt(ctx, desc)
			if err != nil {
				return nil, err
			}
			defer ra.Close()

			dt := make([]byte, ra.Size())
			_, err = ra.ReadAt(dt, 0)
			if err != nil {
				return nil, err
			}

			return NewValue(ctx, string(dt))

		default:
			return nil, Arg(ctx, 0).WithError(fmt.Errorf("has no manifest index"))
		}
	}

	manifest, err := images.Manifest(ctx, resolver, desc, matcher)
	if err != nil {
		return nil, err
	}

	p, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, string(p))
}
