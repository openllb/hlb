package ast

import (
	"context"
	"sync"
)

type modulesKey struct{}

func WithModules(ctx context.Context, mods *ModuleLookup) context.Context {
	return context.WithValue(ctx, modulesKey{}, mods)
}

func Modules(ctx context.Context) *ModuleLookup {
	mods, ok := ctx.Value(modulesKey{}).(*ModuleLookup)
	if !ok {
		return NewModules()
	}
	return mods
}

type ModuleLookup struct {
	mods map[string]*Module
	mu   sync.RWMutex
}

func NewModules() *ModuleLookup {
	return &ModuleLookup{
		mods: make(map[string]*Module),
	}
}

func (ml *ModuleLookup) Get(filename string) *Module {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	return ml.mods[filename]
}

func (ml *ModuleLookup) Set(filename string, mod *Module) {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	ml.mods[filename] = mod
}
