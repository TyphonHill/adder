# Enum strings

This example shows a scan of constants instead of functions, and a template that writes one method per type with `groupBy`. Every `iota` enum in [`order/order.go`](order/order.go) gets a `String` method, in the spirit of `stringer`, without a separate tool or a `//go:generate` line to remember.

```go
type Status int

const (
	StatusPending Status = iota
	StatusPaid
	StatusShipped
	StatusCancelled
)
```

## How it works

- [`adder.yaml`](adder.yaml) declares the `Constants` scan with `values: {kind: const, exported: true}`. Each item's `.Type` is the type the constant is declared with, repeated through its `const` group as Go does, so `StatusPaid` has type `Status` too.
- [`templates/strings.go.tmpl`](templates/strings.go.tmpl) groups the constants with `groupBy "Type" .Constants`, and writes one `String` method per group. Each case trims the type from the constant's name and converts the rest to snake case, so `StatusPending` is `"pending"`.
- The result is [`order/strings.gen.go`](order/strings.gen.go):

```go
// String returns the name of v without its type, in snake case.
func (v Status) String() string {
	switch v {
	case StatusCancelled:
		return "cancelled"
	case StatusPaid:
		return "paid"
	...
	}
	return "Status(" + strconv.Itoa(int(v)) + ")"
}
```

## Try it

Run the commands from this directory, with `go run ../../cmd/adder` in place of `adder` in a checkout of adder.

```console
$ adder check
$ go run ./cmd/demo
shipped high
```

Add `StatusRefunded` to the `Status` group, and `adder derive` adds its case. A new enum type gets a method of its own.
