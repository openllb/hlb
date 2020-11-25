package codegen

import (
	"bytes"
	"fmt"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"
)

func TestTemplateFuncs(t *testing.T) {
	for _, tt := range []struct {
		templateFunc string
		ref          string
		expected     string
	}{
		{"dockerDomain", "docker.io/library/busybox:latest", "docker.io"},
		{"dockerDomain", "docker.io/library/busybox", "docker.io"},
		{"dockerDomain", "docker.io/library/busybox:tag", "docker.io"},
		{"dockerDomain", "busybox:tag", ""},
		{"dockerDomain", "busybox", ""},
		{"dockerDomain", "host.com:8443/library/busybox:tag", "host.com:8443"},
		{"dockerDomain", "host.com:8443/library/busybox", "host.com:8443"},
		{"dockerDomain", "library/busybox:tag", ""},
		{"dockerDomain", "library/busybox", ""},

		{"dockerPath", "docker.io/library/busybox:latest", "library/busybox"},
		{"dockerPath", "docker.io/library/busybox", "library/busybox"},
		{"dockerPath", "docker.io/library/busybox:tag", "library/busybox"},
		{"dockerPath", "busybox:tag", "busybox"},
		{"dockerPath", "busybox", "busybox"},
		{"dockerPath", "host.com:8443/library/busybox:tag", "library/busybox"},
		{"dockerPath", "host.com:8443/library/busybox", "library/busybox"},
		{"dockerPath", "library/busybox:tag", "library/busybox"},
		{"dockerPath", "library/busybox", "library/busybox"},

		{"dockerRepository", "docker.io/library/busybox:latest", "docker.io/library/busybox"},
		{"dockerRepository", "docker.io/library/busybox", "docker.io/library/busybox"},
		{"dockerRepository", "docker.io/library/busybox:tag", "docker.io/library/busybox"},
		{"dockerRepository", "busybox:tag", "busybox"},
		{"dockerRepository", "busybox", "busybox"},
		{"dockerRepository", "host.com:8443/library/busybox:tag", "host.com:8443/library/busybox"},
		{"dockerRepository", "host.com:8443/library/busybox", "host.com:8443/library/busybox"},
		{"dockerRepository", "library/busybox:tag", "library/busybox"},
		{"dockerRepository", "library/busybox", "library/busybox"},

		{"dockerTag", "docker.io/library/busybox:latest", "latest"},
		{"dockerTag", "docker.io/library/busybox", "latest"},
		{"dockerTag", "docker.io/library/busybox:tag", "tag"},
		{"dockerTag", "busybox:tag", "tag"},
		{"dockerTag", "busybox", "latest"},
		{"dockerTag", "host.com:8443/library/busybox:tag", "tag"},
		{"dockerTag", "host.com:8443/library/busybox", "latest"},
		{"dockerTag", "library/busybox:tag", "tag"},
		{"dockerTag", "library/busybox", "latest"},
	} {
		tmpl, err := template.New("hlb").Funcs(templateFuncs()).Parse(
			fmt.Sprintf(`{{%s .}}`, tt.templateFunc),
		)
		require.NoError(t, err)
		buf := bytes.NewBufferString("")
		err = tmpl.Execute(buf, tt.ref)
		require.NoError(t, err)
		require.Equal(t, tt.expected, buf.String(), fmt.Sprintf("{{%s %q}} == %q", tt.templateFunc, tt.ref, tt.expected))
	}
}
