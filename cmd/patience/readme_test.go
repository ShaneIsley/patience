package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestReadme runs every ```sh block in README.md with the freshly built
// binary first on PATH and checks the exit status against a
// "# expect: exit N" line (default 0). Blocks fenced as ```text are prose.
func TestReadme(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```sh\n(.*?)```").FindAllStringSubmatch(string(src), -1)
	if len(blocks) == 0 {
		t.Fatal("no ```sh blocks found in README.md")
	}
	expectRe := regexp.MustCompile(`(?m)^# expect: exit (\d+)$`)
	pathEnv := "PATH=" + filepath.Dir(bin) + string(os.PathListSeparator) + os.Getenv("PATH")

	for i, b := range blocks {
		script := b[1]
		want := 0
		if m := expectRe.FindStringSubmatch(script); m != nil {
			want, _ = strconv.Atoi(m[1])
		}
		first := strings.SplitN(strings.TrimSpace(script), "\n", 2)[0]
		t.Run(strconv.Itoa(i)+"_"+first, func(t *testing.T) {
			cmd := exec.Command("sh", "-c", script)
			cmd.Env = append(os.Environ(), pathEnv)
			out, err := cmd.CombinedOutput()
			got := 0
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				got = ee.ExitCode()
			} else if err != nil {
				t.Fatalf("%v", err)
			}
			if got != want {
				t.Errorf("exit %d, want %d\n--- script ---\n%s--- output ---\n%s", got, want, script, out)
			}
		})
	}
}
