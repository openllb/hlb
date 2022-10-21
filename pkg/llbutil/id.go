package llbutil

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/moby/buildkit/client/llb"
	digest "github.com/opencontainers/go-digest"
)

// LocalID returns a consistent hash for this local (path + options) so that
// the same content doesn't get transported multiple times when referenced
// repeatedly.
func LocalID(ctx context.Context, absPath string, opts ...llb.LocalOption) (string, error) {
	uniqID, err := localUniqueID(absPath)
	if err != nil {
		return "", err
	}
	opts = append(opts, llb.LocalUniqueID(uniqID))
	st := llb.Local("", opts...)

	def, err := st.Marshal(ctx)
	if err != nil {
		return "", err
	}

	// The terminal op of the graph def.Def[len(def.Def)-1] is an empty vertex with
	// an input to the last vertex's digest. Since that vertex also has its digests
	// of its inputs and so on, the digest of the terminal op is a merkle hash for
	// the graph.
	return digest.FromBytes(def.Def[len(def.Def)-1]).String(), nil
}

// localUniqueID returns a consistent string that is unique per host + dir +
// last modified time.
//
// If there is already a solve in progress using the same local dir, we want to
// deduplicate the "local" if the directory hasn't changed, but if there has
// been a change, we must not identify the "local" as a duplicate. Thus, we
// incorporate the last modified timestamp into the result.
func localUniqueID(dir string) (string, error) {
	mac, err := FirstUpInterface()
	if err != nil {
		return "", err
	}

	var lastModified time.Time
	err = filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if lastModified.IsZero() || info.ModTime().After(lastModified) {
			lastModified = info.ModTime()
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("dir:%s,mac:%s,modified:%s", dir, mac, lastModified.Format(time.RFC3339Nano)), nil
}

// FirstUpInterface returns the mac address for the first "UP" network
// interface.
func FirstUpInterface() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // not up
		}
		if iface.HardwareAddr.String() == "" {
			continue // no mac
		}
		return iface.HardwareAddr.String(), nil

	}
	return "no-valid-interface", nil
}

func SecretID(path string) string {
	return digest.FromString(path).String()
}

func SSHID(paths ...string) string {
	return digest.FromString(strings.Join(paths, "")).String()
}

func OutputFromWriter(w io.WriteCloser) func(map[string]string) (io.WriteCloser, error) {
	return func(map[string]string) (io.WriteCloser, error) {
		return w, nil
	}
}
