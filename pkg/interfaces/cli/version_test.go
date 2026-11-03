package cli

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"github.com/nonchan7720/spec-gate/pkg/version"
)

// setBuildInfo は ldflags で埋め込まれる値をテストの間だけ差し替える。
func setBuildInfo(t *testing.T, v, commit, date string) {
	t.Helper()
	oldV, oldC, oldD := version.Version, version.Commit, version.Date
	version.Version, version.Commit, version.Date = v, commit, date
	t.Cleanup(func() { version.Version, version.Commit, version.Date = oldV, oldC, oldD })
}

func TestVersionCommand(t *testing.T) {
	setBuildInfo(t, "1.2.3", "abc1234", "2026-01-02T03:04:05Z")
	cmd := newVersionCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"spec-gate 1.2.3", "commit: abc1234", "built:  2026-01-02T03:04:05Z", runtime.Version()} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("出力に %q が無い:\n%s", want, out.String())
		}
	}
}

func TestVersionTemplate(t *testing.T) {
	setBuildInfo(t, "1.2.3", "abc1234", "2026-01-02T03:04:05Z")
	got := versionTemplate()
	want := "spec-gate {{.Version}} (commit abc1234, built 2026-01-02T03:04:05Z)\n"
	if got != want {
		t.Fatalf("versionTemplate() = %q, want %q", got, want)
	}
}
