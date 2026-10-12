package run

import "testing"

func TestNormalizeMatchesPythonConvertTail(t *testing.T) {
	in := "a \r\nb  \r\n\r\n\r\nc\n\n\n\nd"
	got, cut := Cut(in, 1<<20)
	if cut || got != "a\nb\n\nc\n\nd" {
		t.Fatalf("cut=%v got=%q", cut, got)
	}
	got, cut = Cut("x\n\n\n", 1<<20)
	if cut || got != "x\n\n" {
		t.Fatalf("cut=%v got=%q", cut, got)
	}
	got, cut = Cut("  hi  \n", 1<<20)
	if cut || got != "  hi\n" {
		t.Fatalf("cut=%v got=%q", cut, got)
	}
	got, cut = Cut("éé", 3)
	if !cut || got != "é" {
		t.Fatalf("cut=%v got=%q", cut, got)
	}
}
