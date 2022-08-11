package solver

import (
	"strings"
	"sync"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	digest "github.com/opencontainers/go-digest"
)

// MultiWriter is similar to progress.MultiWriter, but deduplicates writes by
// vertex.
type MultiWriter struct {
	w                  progress.Writer
	allClaimedVertices map[digest.Digest]struct{}
	claimedVerticesMu  sync.Mutex
}

func NewMultiWriter(pw progress.Writer, prefix ...string) *MultiWriter {
	if pw == nil {
		return nil
	}

	for _, p := range prefix {
		if p != "" {
			pw = progress.WithPrefix(pw, p, true)
		}
	}

	return &MultiWriter{
		w:                  pw,
		allClaimedVertices: make(map[digest.Digest]struct{}),
	}
}

func (mw *MultiWriter) WithPrefix(pfx string, force bool) progress.Writer {
	return &prefixed{
		mw:              mw,
		pfx:             pfx,
		force:           force,
		claimedVertices: make(map[digest.Digest]struct{}),
	}
}

type prefixed struct {
	mw              *MultiWriter
	pfx             string
	force           bool
	claimedVertices map[digest.Digest]struct{}
}

func (p *prefixed) Write(v *client.SolveStatus) {
	filtered := &client.SolveStatus{
		Vertexes: v.Vertexes,
		Statuses: v.Statuses,
	}

	p.mw.claimedVerticesMu.Lock()
	for _, log := range v.Logs {
		if _, ourVertex := p.claimedVertices[log.Vertex]; ourVertex {
			filtered.Logs = append(filtered.Logs, log)
			continue
		}
		if _, claimed := p.mw.allClaimedVertices[log.Vertex]; !claimed {
			p.mw.allClaimedVertices[log.Vertex] = struct{}{}
			p.claimedVertices[log.Vertex] = struct{}{}
			filtered.Logs = append(filtered.Logs, log)
		}
	}
	p.mw.claimedVerticesMu.Unlock()

	if p.force {
		for _, v := range filtered.Vertexes {
			v.Name = addPrefix(p.pfx, v.Name)
		}
	}
	p.mw.w.Write(filtered)
}

func (p *prefixed) ValidateLogSource(dgst digest.Digest, v interface{}) bool {
	return p.mw.w.ValidateLogSource(dgst, v)
}

func (p *prefixed) ClearLogSource(v interface{}) {
	p.mw.w.ClearLogSource(v)
}

func addPrefix(pfx, name string) string {
	if strings.HasPrefix(name, "[") {
		return "[" + pfx + " " + name[1:]
	}
	return "[" + pfx + "] " + name
}
