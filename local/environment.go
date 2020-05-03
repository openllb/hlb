package local

import (
	"context"
	"os"
	"runtime"
	"strings"
)

type contextKey string

const (
	environContextKey contextKey = "environ"
	cwdContextKey     contextKey = "cwd"
	osContextKey      contextKey = "os"
	archContextKey    contextKey = "arch"
)

func WithEnviron(ctx context.Context, environ []string) context.Context {
	if environ == nil {
		return ctx
	}
	return context.WithValue(ctx, environContextKey, environ)
}

func WithCwd(ctx context.Context, cwd string) (context.Context, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return ctx, err
		}
	}
	return context.WithValue(ctx, cwdContextKey, cwd), nil
}

func WithOs(ctx context.Context, os string) context.Context {
	if os == "" {
		os = runtime.GOOS
	}
	return context.WithValue(ctx, osContextKey, os)
}

func WithArch(ctx context.Context, arch string) context.Context {
	if arch == "" {
		arch = runtime.GOARCH
	}
	return context.WithValue(ctx, archContextKey, arch)
}

func Env(ctx context.Context, key string) string {
	if environ, ok := ctx.Value(environContextKey).([]string); ok {
		for _, env := range environ {
			envParts := strings.SplitN(env, "=", 2)
			if envParts[0] == key {
				if len(envParts) > 1 {
					return envParts[1]
				}
				return ""
			}
		}
		// did not find the key
		return ""
	}
	return os.Getenv(key)
}

func Environ(ctx context.Context) []string {
	if environ, ok := ctx.Value(environContextKey).([]string); ok {
		return environ
	}
	return os.Environ()
}

func Cwd(ctx context.Context) (string, error) {
	if workdir, ok := ctx.Value(cwdContextKey).(string); ok {
		return workdir, nil
	}
	return os.Getwd()
}

func Os(ctx context.Context) string {
	if os, ok := ctx.Value(osContextKey).(string); ok {
		return os
	}
	return runtime.GOOS
}

func Arch(ctx context.Context) string {
	if os, ok := ctx.Value(archContextKey).(string); ok {
		return os
	}
	return runtime.GOARCH
}
