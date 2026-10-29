// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Adapted from the Go toolchain's src/internal/diff/diff_test.go, with a small txtar reader in place of
// internal/txtar.

package diff

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clean(text []byte) []byte {
	text = bytes.ReplaceAll(text, []byte("$\n"), []byte("\n"))
	text = bytes.TrimSuffix(text, []byte("^D\n"))
	return text
}

// file is one file of a txtar archive.
type file struct {
	name string
	data []byte
}

// parseTxtar reads the files of a txtar archive, the format of the testdata, ignoring its leading comment.
func parseTxtar(data []byte) []file {
	var files []file
	for line := range strings.Lines(string(data)) {
		if name, ok := strings.CutPrefix(strings.TrimSuffix(line, "\n"), "-- "); ok && strings.HasSuffix(name, " --") {
			files = append(files, file{name: strings.TrimSuffix(name, " --")})
			continue
		}
		if len(files) > 0 {
			files[len(files)-1].data = append(files[len(files)-1].data, line...)
		}
	}
	return files
}

func Test(t *testing.T) {
	names, _ := filepath.Glob("testdata/*.txt")
	if len(names) == 0 {
		t.Fatalf("no testdata")
	}

	for _, name := range names {
		t.Run(filepath.Base(name), func(t *testing.T) {
			data, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			files := parseTxtar(data)
			if len(files) != 3 || files[2].name != "diff" {
				t.Fatalf("%s: want three files, third named \"diff\"", name)
			}
			diffs := Diff(files[0].name, clean(files[0].data), files[1].name, clean(files[1].data))
			want := clean(files[2].data)
			if !bytes.Equal(diffs, want) {
				t.Fatalf("%s: have:\n%s\nwant:\n%s\n%s", name,
					diffs, want, Diff("have", diffs, "want", want))
			}
		})
	}
}
