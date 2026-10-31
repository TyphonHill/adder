# List SQL migrations

This example shows a file scan, which lists files instead of Go declarations. Every SQL migration in [`migrations/`](migrations) is listed, in name order, in a Go file the application reads them through. Adding a migration is a matter of adding its `.sql` file.

## How it works

- [`migrations/`](migrations) holds the migrations, named so that name order is the order they apply in, and a `README.txt` that is not a migration.
- [`adder.yaml`](adder.yaml) declares the `Migrations` scan with `files: {name: \.sql$}`, which selects files whose name ends in `.sql`. Each item's `.Name` is the file name, and `.File` its path from the repository root.
- [`templates/migrations.go.tmpl`](templates/migrations.go.tmpl) writes the names to [`migrations/migrations.gen.go`](migrations/migrations.gen.go). Every scan sorts its items by name, so the order is the same on every machine:

```go
var Names = []string{
	"0001_create_users.sql",
	"0002_create_orders.sql",
	"0003_add_order_status.sql",
}
```

- [`migrations/embed.go`](migrations/embed.go) embeds the SQL with `//go:embed`, written by hand. The derived list says which files there are and in what order. The embedded files hold their contents.

## Try it

Run the commands from this directory, with `go run ../../cmd/adder` in place of `adder` in a checkout of adder.

```console
$ adder check
$ go run ./cmd/migrate
-- 0001_create_users.sql
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
-- 0002_create_orders.sql
...
```

Add `migrations/0004_create_payments.sql`, and `adder check` reports the list as out of date until `adder derive` adds it.
