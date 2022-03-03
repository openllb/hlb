package dapserver

import (
	"bufio"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"log"

	"github.com/chzyer/readline"
	dap "github.com/google/go-dap"
	"github.com/openllb/hlb/codegen"
	"golang.org/x/sync/errgroup"
)

type Server struct {
	dbgr codegen.Debugger
}

func New(dbgr codegen.Debugger) *Server {
	return &Server{dbgr}
}

func (s *Server) Listen(ctx context.Context, output, stdin io.Reader, stdout io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	cancelableStdin := readline.NewCancelableStdin(stdin)
	session := Session{
		dbgr: s.dbgr,
		rw: bufio.NewReadWriter(
			bufio.NewReader(cancelableStdin),
			bufio.NewWriter(stdout),
		),
		cancel:            cancel,
		sendQueue:         make(chan dap.Message),
		caps:              make(map[Capability]struct{}),
		sourcesHandles:    newHandlesMap(),
		variablesHandles:  newHandlesMap(),
		stackFrameHandles: newHandlesMap(),
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return session.sendFromQueue(ctx)
	})

	if output == nil {
		g.Go(func() error {
			<-ctx.Done()
			return cancelableStdin.Close()
		})
	} else {
		g.Go(func() error {
			defer cancelableStdin.Close()

			scanner := bufio.NewScanner(output)
			for scanner.Scan() {
				session.send(&dap.OutputEvent{
					Event: newEvent("output"),
					Body: dap.OutputEventBody{
						Category: "stdout",
						Output:   scanner.Text() + "\n",
					},
				})
				select {
				case <-ctx.Done():
					return nil
				default:
				}
			}

			return scanner.Err()
		})
	}

	// f, err := os.Create("/tmp/hlb-dapserver.log")
	// if err != nil {
	// 	panic(err)
	// }
	// defer f.Close()
	// log.SetOutput(f)

	log.SetOutput(ioutil.Discard)

	log.Printf("Listening on stdio")
	g.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			if err := session.handleRequest(ctx); err != nil {
				return err
			}
		}
	})

	session.sendWg.Wait()
	if err := g.Wait(); !errors.Is(err, io.EOF) {
		return err
	}
	return session.err
}
