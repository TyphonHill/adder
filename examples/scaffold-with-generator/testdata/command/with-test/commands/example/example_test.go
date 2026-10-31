package example

import (
	"bytes"
	"testing"
)

func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := Run(&out, nil); err != nil {
		t.Fatal(err)
	}
}
