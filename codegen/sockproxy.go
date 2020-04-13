package codegen

import (
	"io"
	"net"
)

func RunSockProxy(conn net.Conn, l net.Listener) error {
	for {
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
