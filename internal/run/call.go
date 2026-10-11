package run

import (
	"path"
	"strings"
)

// Ext is the lower-case extension of filename, including the dot.
func Ext(filename string) string {
	filename = strings.ReplaceAll(filename, "\\", "/")
	return strings.ToLower(path.Ext(path.Base(filename)))
}
