package diag

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"testing"
)

func TestPosString(t *testing.T) {
	tests := []struct {
		name string
		pos  Pos
		want string
	}{
		{name: "unknown", pos: Pos{}, want: ""},
		{name: "file only", pos: Pos{File: "adder.yaml"}, want: "adder.yaml"},
		{name: "file and line", pos: Pos{File: "adder.yaml", Line: 3}, want: "adder.yaml:3"},
		{name: "file, line, and column", pos: Pos{File: "adder.yaml", Line: 3, Column: 7}, want: "adder.yaml:3:7"},
		{name: "column without line", pos: Pos{File: "adder.yaml", Column: 7}, want: "adder.yaml"},
		{name: "line and column without file", pos: Pos{Line: 3, Column: 7}, want: "3:7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pos.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDiagnostic(t *testing.T) {
	tests := []struct {
		name       string
		d          *Diagnostic
		wantError  string
		wantUnwrap error
	}{
		{
			name:      "with position",
			d:         Errorf(MissingField, Pos{File: "adder.yaml", Line: 2}, "%s is required", "path"),
			wantError: "adder.yaml:2: path is required",
		},
		{
			name:      "without position",
			d:         Errorf(Other, Pos{}, "plain"),
			wantError: "plain",
		},
		{
			name:       "wrapped error",
			d:          Wrap(IO, Pos{File: "x"}, fs.ErrNotExist),
			wantError:  "x: " + fs.ErrNotExist.Error(),
			wantUnwrap: fs.ErrNotExist,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.Error(); got != tt.wantError {
				t.Errorf("Error() = %q, want %q", got, tt.wantError)
			}
			if got := tt.d.Unwrap(); !errors.Is(got, tt.wantUnwrap) || (got == nil) != (tt.wantUnwrap == nil) {
				t.Errorf("Unwrap() = %v, want %v", got, tt.wantUnwrap)
			}
		})
	}
}

func TestListErr(t *testing.T) {
	tests := []struct {
		name string
		list List
		want string // empty means Err returns nil
	}{
		{name: "nil", list: nil},
		{name: "empty", list: List{}},
		{name: "one", list: List{Errorf(Other, Pos{}, "one")}, want: "one"},
		{name: "several", list: List{Errorf(Other, Pos{}, "one"), Errorf(Other, Pos{File: "f"}, "two")}, want: "one\nf: two"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.list.Err()
			if tt.want == "" {
				if err != nil {
					t.Errorf("Err() = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != tt.want {
				t.Errorf("Err() = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestFrom(t *testing.T) {
	d := Errorf(NoMatch, Pos{}, "nothing")
	plain := errors.New("plain")
	tests := []struct {
		name      string
		err       error
		wantCodes []Code
		wantFirst *Diagnostic // when set, the first diagnostic must be this value
		wantIs    error
	}{
		{name: "nil", err: nil},
		{name: "wrapped list", err: fmt.Errorf("wrapped: %w", List{d, d}), wantCodes: []Code{NoMatch, NoMatch}},
		{name: "wrapped diagnostic", err: fmt.Errorf("wrapped: %w", d), wantCodes: []Code{NoMatch}, wantFirst: d},
		{name: "plain error", err: plain, wantCodes: []Code{Other}, wantIs: plain},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := From(tt.err)
			if len(got) != len(tt.wantCodes) {
				t.Fatalf("From() = %v, want %d diagnostics", got, len(tt.wantCodes))
			}
			for i, code := range tt.wantCodes {
				if got[i].Code != code {
					t.Errorf("diagnostic %d code = %s, want %s", i, got[i].Code, code)
				}
			}
			if tt.wantFirst != nil && got[0] != tt.wantFirst {
				t.Errorf("first diagnostic = %p, want %p", got[0], tt.wantFirst)
			}
			if tt.wantIs != nil && !errors.Is(got[0], tt.wantIs) {
				t.Errorf("first diagnostic does not wrap %v", tt.wantIs)
			}
		})
	}
}

func TestDiagnosticJSON(t *testing.T) {
	tests := []struct {
		name string
		d    *Diagnostic
		want string
	}{
		{
			name: "every field",
			d:    &Diagnostic{Code: UnknownField, Message: `unknown field "scanz"`, Pos: Pos{File: "adder.yaml", Line: 4}, Hint: "known fields: scans, outputs", Err: errors.New("hidden")},
			want: `{"code":"unknown-field","message":"unknown field \"scanz\"","pos":{"file":"adder.yaml","line":4},"hint":"known fields: scans, outputs"}`,
		},
		{
			name: "no position or hint",
			d:    &Diagnostic{Code: Other, Message: "no position"},
			want: `{"code":"error","message":"no position"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.d)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("json = %s, want %s", got, tt.want)
			}
		})
	}
}
