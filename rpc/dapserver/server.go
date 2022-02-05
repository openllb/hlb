package dapserver

import (
	"bufio"
	"context"
	"io"
	"log"

	dap "github.com/google/go-dap"
	"github.com/openllb/hlb/codegen"
)

type Server struct {
	debugger codegen.Debugger
}

func New(debugger codegen.Debugger) *Server {
	return &Server{debugger}
}

func (s *Server) Listen(ctx context.Context, r io.Reader, w io.Writer) error {
	session := Session{
		debugger: s.debugger,
		rw: bufio.NewReadWriter(
			bufio.NewReader(r),
			bufio.NewWriter(w),
		),
		sendQueue:         make(chan dap.Message),
		caps:              make(map[Capability]struct{}),
		sourcesHandles:    newHandlesMap(),
		variablesHandles:  newHandlesMap(),
		stackFrameHandles: newHandlesMap(),
	}
	go session.sendFromQueue()

	log.Printf("Listening on stdio")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-session.done:
			break
		default:
		}
		err := session.handleRequest(ctx)
		if err != nil {
			log.Printf("handleRequest err: %s", err)
			if err == io.EOF {
				break
			}
			return err
		}
	}

	session.sendWg.Wait()
	close(session.sendQueue)
	return session.err
}
