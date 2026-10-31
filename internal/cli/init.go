package cli

import (
	"cmp"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"typhonhill.dev/adder/internal/config"
	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/plan"
)

// starterConfig is what adder init writes: a valid configuration whose commented examples are valid too.
const starterConfig = `# yaml-language-server: $schema=https://typhonhill.dev/adder/schema/adder-v1.json
# adder configuration: https://github.com/typhonhilldev/adder/blob/main/docs/configuration.md
version: 1

# vars are strings every template can read as .Vars.<name>.
# vars:
#   License: Apache-2.0

# derive rebuilds files from scans of the repository's Go code. Run adder derive to update them,
# and adder check in CI to catch outputs that are out of date.
derive:
  scans:
    # Each scan collects matching declarations, which templates read by the scan's name.
    # Handlers:
    #   dirs: ["internal/handlers/*"]
    #   funcs: {exported: true, name: ^Handle}
  outputs:
    # - template: templates/handlers.go.tmpl
    #   path: internal/handlers/handlers.gen.go

# generators create new files from templates. Run adder new <name>.
# generators:
#   handler:
#     description: Add an HTTP handler
#     inputs:
#       name: {help: Handler name, pattern: "[a-z][a-z0-9]*", unique: Handlers.Package}
#     files:
#       - template: templates/new/handler.go.tmpl
#         path: internal/handlers/{{ .Inputs.name }}/{{ .Inputs.name }}.go
`

func newInitCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create a starter adder.yaml",
		Long: `init writes a starter adder.yaml to the repository root, the current directory unless --root or
--config says otherwise. Its commented examples show each section. An existing file is never overwritten.`,
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			rep := &planReport{Changes: []changeReport{}}
			opts.report = rep
			path := cmp.Or(opts.config, filepath.Join(cmp.Or(opts.root, "."), config.FileName))
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if errors.Is(err, fs.ErrExist) {
				return &diag.Diagnostic{Code: diag.FileExists, Pos: diag.Pos{File: path}, Message: "file already exists", Hint: "adder never overwrites it; edit it instead"}
			}
			if err != nil {
				return diag.Wrap(diag.IO, diag.Pos{File: path}, err)
			}
			_, err = f.WriteString(starterConfig)
			if err := errors.Join(err, f.Close()); err != nil {
				return diag.Wrap(diag.IO, diag.Pos{File: path}, err)
			}
			rep.Changes = []changeReport{{Path: filepath.ToSlash(path), Status: plan.Created}}
			if !opts.json {
				opts.printer(cmd.OutOrStdout()).change(plan.Change{Path: filepath.ToSlash(path), Status: plan.Created}, false)
			}
			return nil
		},
	}
}
