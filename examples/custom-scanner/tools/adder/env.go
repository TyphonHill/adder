package main

import (
	"context"
	"io/fs"
	"path"
	"strings"

	"typhonhill.dev/adder"
)

// envFile lists the settings in the files a scan names, written KEY=value one per line, with # comments.
func envFile(_ context.Context, in *adder.ScanInput) ([]any, error) {
	var opts struct {
		Files []string `yaml:"files"`
	}
	if err := in.Decode(&opts); err != nil {
		return nil, err
	}
	var items []any
	for _, file := range opts.Files {
		data, err := fs.ReadFile(in.FS, file)
		if err != nil {
			return nil, err
		}
		for i, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok || key == "" {
				return nil, adder.Problem{File: file, Line: i + 1, Message: "want KEY=value", Hint: "comment lines start with #"}
			}
			items = append(items, map[string]any{"Key": key, "Value": value, "File": path.Base(file)})
		}
	}
	return items, nil
}
