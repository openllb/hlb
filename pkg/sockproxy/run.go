package sockproxy

import (
	"context"
	"io"
	"net"
)

func Run(ctx context.Context, conn net.Conn, l net.Listener) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		proxy, err := l.Accept()
		if err != nil {
			return err
		}

		go func() {
			defer proxy.Close()
			io.Copy(conn, proxy)
		}()

		go func() {
			io.Copy(proxy, conn)
		}()
	}
}
