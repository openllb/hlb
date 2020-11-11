package parser

import (
	"sort"
)

type KindSet struct {
	set map[Kind]struct{}
}

func NewKindSet(kinds ...Kind) *KindSet {
	set := make(map[Kind]struct{})
	for _, kind := range kinds {
		if kind.Primary() == Option {
			set[Option] = struct{}{}
		}
		set[kind] = struct{}{}
	}
	return &KindSet{set}
}

func (ks *KindSet) Has(kind Kind) bool {
	_, ok := ks.set[kind]
	return ok
}

func (ks *KindSet) Kinds() (kinds []Kind) {
	for kind := range ks.set {
		kinds = append(kinds, kind)
	}
	sort.SliceStable(kinds, func(i, j int) bool {
		return kinds[i] < kinds[j]
	})
	return kinds
}
