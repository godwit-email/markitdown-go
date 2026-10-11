package markitdown

import (
	"strings"
	"testing"
)

func TestCSVRecordSeparators(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n", "\r"} {
		for _, final := range []bool{false, true} {
			content := strings.Join([]string{"name,age", "Alice,30"}, nl)
			if final {
				content += nl
			}
			got := mustCSV(t, content)
			want := "| name | age |\n| --- | --- |\n| Alice | 30 |"
			if got != want {
				t.Fatalf("nl %q final %v\n%s", nl, final, got)
			}
		}
	}
}

func TestCSVQuotedLineBreak(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n", "\r"} {
		for _, cell := range []string{"\n", "\r\n", "\r"} {
			content := strings.Join([]string{"name,notes", `Alice,"first` + cell + `second"`, "Bob,plain"}, nl)
			got := mustCSV(t, content)
			want := "| name | notes |\n| --- | --- |\n| Alice | first second |\n| Bob | plain |"
			if got != want {
				t.Fatalf("%q", got)
			}
		}
	}
}

func TestCSVLongBlankRuns(t *testing.T) {
	blank := strings.Repeat("\n", 1000)
	header := "name,value\n"
	data := "Alice,1\n\nBob,2,extra\n"
	want := "| name | value |  |\n| --- | --- | --- |\n| Alice | 1 |  |\n|  |  |  |\n| Bob | 2 | extra |"
	cases := map[string]string{
		"leading":      blank + header + data,
		"after_header": header + blank + data,
		"trailing":     header + data + blank,
		"all":          blank,
	}
	for name, content := range cases {
		got := mustCSV(t, content)
		exp := want
		if name == "all" {
			exp = ""
		}
		if got != exp {
			t.Fatalf("%s\n%s", name, got)
		}
	}
}

func TestCSVBlankLineRules(t *testing.T) {
	cases := []struct{ in, want string }{
		{"\ufeffname,age\nAlice,30\n", "| name | age |\n| --- | --- |\n| Alice | 30 |"},
		{"\nname,age\nAlice,30\n", "| name | age |\n| --- | --- |\n| Alice | 30 |"},
		{"name,age\nAlice,30\n\n\n", "| name | age |\n| --- | --- |\n| Alice | 30 |"},
		{"name,age\n\nAlice,30\n\nBob,40\n", "| name | age |\n| --- | --- |\n| Alice | 30 |\n|  |  |\n| Bob | 40 |"},
		{"\n\n\n", ""},
		{"banner\nname,age,city\nAlice,30,Seattle\n", "| banner |  |  |\n| --- | --- | --- |\n| name | age | city |\n| Alice | 30 | Seattle |"},
		{"name,age\nAlice\n\nBob,40,Seattle\nCarol,25\n", "| name | age |  |\n| --- | --- | --- |\n| Alice |  |  |\n|  |  |  |\n| Bob | 40 | Seattle |\n| Carol | 25 |  |"},
		{"name\nAlice,,\n", "| name |  |  |\n| --- | --- | --- |\n| Alice |  |  |"},
		{"name,age,city\nAlice,30\nBob\n", "| name | age | city |\n| --- | --- | --- |\n| Alice | 30 |  |\n| Bob |  |  |"},
	}
	for _, tc := range cases {
		got := mustCSV(t, tc.in)
		if got != tc.want {
			t.Fatalf("in %q\ngot  %q\nwant %q", tc.in, got, tc.want)
		}
	}
}

func TestCSVPipeEscape(t *testing.T) {
	if !strings.Contains(mustCSV(t, "name,description\nWidget,\"cheap | fast\"\n"), "| Widget | cheap \\| fast |") {
		t.Fatal(mustCSV(t, "name,description\nWidget,\"cheap | fast\"\n"))
	}
	if !strings.Contains(mustCSV(t, "name,description\nWidget,\"left\\|right\"\n"), `| Widget | left\\\|right |`) {
		t.Fatal(mustCSV(t, "name,description\nWidget,\"left\\|right\"\n"))
	}
	if !strings.Contains(mustCSV(t, "name,description\nWidget,\"left\\\\|right\"\n"), `| Widget | left\\\\\|right |`) {
		t.Fatal(mustCSV(t, "name,description\nWidget,\"left\\\\|right\"\n"))
	}
	got := mustCSV(t, "name,notes\nWidget,\"line one\nline two\"\n")
	if strings.Count(got, "\n") != 2 || !strings.Contains(got, "| Widget | line one line two |") {
		t.Fatal(got)
	}
	got = mustCSV(t, "name,notes\nWidget,\"line one\r\nline two\rline three\"\n")
	if strings.Count(got, "\n") != 2 || !strings.Contains(got, "| Widget | line one line two line three |") {
		t.Fatal(got)
	}
	if !strings.Contains(mustCSV(t, "\"a | b\",c\n1,2\n"), "| a \\| b | c |") {
		t.Fatal("header")
	}
	got = mustCSV(t, "name,description\nWidget,cheap and fast\n")
	if !strings.Contains(got, "| Widget | cheap and fast |") || strings.Contains(got, `\`) {
		t.Fatal(got)
	}
	if !strings.Contains(mustCSV(t, "name,path\nWidget,C:\\temp\\file.txt\n"), `| Widget | C:\temp\file.txt |`) {
		t.Fatal(mustCSV(t, "name,path\nWidget,C:\\temp\\file.txt\n"))
	}
}

func TestCSVLongBackslashRun(t *testing.T) {
	backslashes := strings.Repeat(`\`, 1024)
	for _, suffix := range []string{"", "|", "x|"} {
		value := backslashes + suffix
		escaped := backslashes
		if suffix == "|" {
			escaped = backslashes + backslashes + `\|`
		} else {
			escaped = backslashes + strings.ReplaceAll(suffix, "|", `\|`)
		}
		got := mustCSV(t, value+"\n"+value+"\n")
		want := "| " + escaped + " |\n| --- |\n| " + escaped + " |"
		if got != want {
			t.Fatalf("suffix %q len got %d want %d", suffix, len(got), len(want))
		}
	}
}

func mustCSV(t *testing.T, content string) string {
	t.Helper()
	res, err := Extract(strings.NewReader(content), "text/csv; charset=utf-8", "a.csv", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindCSV || res.Partial {
		t.Fatalf("%#v", res)
	}
	return res.Text
}
