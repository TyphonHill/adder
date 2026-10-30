package scaffold

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"maps"
	"path"
	"slices"

	"typhonhill.dev/adder/internal/config"
	"typhonhill.dev/adder/internal/derive"
	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/plan"
)

// TestResult is how a generator test's files compare with its golden files.
type TestResult struct {
	Generator string
	Test      config.GeneratorTest
	// Changes would turn the want directory into what the generator creates now, with paths relative to the
	// configuration file's directory. A test passes when there are none.
	Changes []plan.Change
}

// Passed reports whether the generator creates exactly the files under want.
func (r *TestResult) Passed() bool { return len(r.Changes) == 0 }

// Test runs a test of the generator called name like a dry run against src, and compares the files the
// generator creates with the test's want directory in src.Templates. Derived outputs are not compared, since
// they depend on everything in the repository.
func Test(ctx context.Context, cfg *config.Config, src derive.Sources, name string, test config.GeneratorTest) (*TestResult, error) {
	res, _, err := planFiles(ctx, cfg, src, name, test.Inputs)
	if err != nil {
		return nil, diag.Locate(err, "", test.Pos)
	}
	got := make(map[string][]byte)
	for _, c := range res.Plan.Changes {
		if c.Status == plan.Created {
			got[c.Path] = c.New
		}
	}
	want, err := goldenFiles(src.Templates, test.Want)
	if err != nil {
		return nil, diag.Locate(err, src.TemplatesDir, test.Pos)
	}

	result := &TestResult{Generator: name, Test: test}
	for _, file := range slices.Sorted(maps.Keys(merge(got, want))) {
		gotContent, created := got[file]
		wantContent, golden := want[file]
		c := plan.Change{Path: path.Join(test.Want, file), Old: wantContent, New: gotContent}
		switch {
		case !golden:
			c.Status = plan.Created
		case !created:
			c.Status, c.New = plan.Removed, nil
		case !bytes.Equal(gotContent, wantContent):
			c.Status = plan.Updated
		default:
			continue
		}
		result.Changes = append(result.Changes, c)
	}
	return result, nil
}

// goldenFiles returns the regular files under dir in fsys, by slash-separated path relative to dir. A missing
// dir holds no files.
func goldenFiles(fsys fs.FS, dir string) (map[string][]byte, error) {
	files := make(map[string][]byte)
	err := fs.WalkDir(fsys, dir, func(name string, entry fs.DirEntry, err error) error {
		switch {
		case name == dir && errors.Is(err, fs.ErrNotExist):
			return fs.SkipAll
		case err != nil:
			return diag.Wrap(diag.IO, diag.Pos{File: name}, err)
		case !entry.Type().IsRegular():
			return nil
		}
		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			return diag.Wrap(diag.IO, diag.Pos{File: name}, err)
		}
		rel := name
		if dir != "." {
			rel = name[len(dir)+1:]
		}
		files[rel] = content
		return nil
	})
	return files, err
}

func merge(a, b map[string][]byte) map[string][]byte {
	m := maps.Clone(a)
	maps.Copy(m, b)
	return m
}
