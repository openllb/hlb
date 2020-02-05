package solver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/moby/buildkit/client"
	digest "github.com/opencontainers/go-digest"
)

func StreamSolveStatus(ctx context.Context, logOutput LogOutput, w io.Writer, ch chan *client.SolveStatus) error {
	var done bool
	t := newTrace(w, logOutput)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ss, ok := <-ch:
			if ok {
				err := t.printSolveStatus(ss)
				if err != nil {
					return err
				}
			} else {
				done = true
			}
		}

		if done {
			return nil
		}
	}
}

type trace struct {
	w         io.Writer
	logOutput LogOutput
	byDigest  map[digest.Digest]string
}

func newTrace(w io.Writer, logOutput LogOutput) *trace {
	return &trace{
		w:         w,
		logOutput: logOutput,
		byDigest:  make(map[digest.Digest]string),
	}
}

func (t *trace) printSolveStatus(s *client.SolveStatus) error {
	switch t.logOutput {
	case LogOutputJSON:
		dt, err := json.Marshal(s)
		if err != nil {
			return err
		}

		fmt.Fprint(t.w, string(dt))
	case LogOutputRaw:
		for _, l := range s.Logs {
			fmt.Fprint(t.w, string(l.Data))
		}
	}
	return nil
}
