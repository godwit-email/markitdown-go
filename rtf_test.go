package markitdown

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestSamplePluginRTF(t *testing.T) {
	body, err := os.ReadFile("packages/markitdown-sample-plugin/tests/test_files/test.rtf")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"This is a Sample RTF File",
		"It is included to test if the MarkItDown sample plugin can correctly convert RTF files.",
	}
	for _, tc := range []struct{ mime, name string }{
		{"text/rtf", "test.rtf"},
		{"application/rtf", "test.rtf"},
		{"", "test.rtf"},
	} {
		res, err := Extract(bytes.NewReader(body), tc.mime, tc.name, 0)
		if err != nil {
			t.Fatal(tc.mime, err)
		}
		if res.Kind != KindRTF {
			t.Fatal(res.Kind)
		}
		for _, s := range want {
			if !strings.Contains(res.Text, s) {
				t.Fatalf("mime %q missing %q\n%s", tc.mime, s, res.Text)
			}
		}
	}
}
