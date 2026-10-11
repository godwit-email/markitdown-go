package empty_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/godwit-email/markitdown-go"
)

func TestShellRegistersNothing(t *testing.T) {
	if got := markitdown.Kinds(); len(got) != 0 {
		t.Fatalf("Kinds = %v", got)
	}
	for _, tc := range []struct{ mime, name, body string }{
		{"text/plain", "a.txt", "hello"},
		{"application/pdf", "a.pdf", "%PDF-1.4"},
		{"", "a.csv", "a,b\n1,2\n"},
	} {
		_, err := markitdown.Extract(context.Background(), markitdown.Request{
			R: strings.NewReader(tc.body), MIME: tc.mime, Filename: tc.name,
		})
		if !errors.Is(err, markitdown.ErrUnsupported) {
			t.Fatalf("%s %s: %v", tc.mime, tc.name, err)
		}
	}
	_, err := markitdown.Extract(context.Background(), markitdown.Request{
		R: bytes.NewReader(bytes.Repeat([]byte{'x'}, 8)), MIME: "", Filename: "a.pdf",
	})
	if !errors.Is(err, markitdown.ErrUnsupported) {
		t.Fatal(err)
	}
}
