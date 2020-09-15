package checker

import (
	"sort"

	"github.com/openllb/hlb/parser"
)

type KindSet struct {
	set map[parser.Kind]struct{}
}

func NewKindSet(kinds ...parser.Kind) *KindSet {
	set := make(map[parser.Kind]struct{})
	for _, kind := range kinds {
		if kind.Primary() == parser.Option {
			set[parser.Option] = struct{}{}
		}
		set[kind] = struct{}{}
	}
	return &KindSet{set}
}

func (ks *KindSet) Has(kind parser.Kind) bool {
	_, ok := ks.set[kind]
	return ok
}

func (ks *KindSet) Kinds() (kinds []parser.Kind) {
	for kind := range ks.set {
		kinds = append(kinds, kind)
	}
	sort.SliceStable(kinds, func(i, j int) bool {
		return kinds[i] < kinds[j]
	})
	return kinds
}
