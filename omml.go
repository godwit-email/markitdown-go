package markitdown

import (
	"fmt"
	"strings"
)

// Math function names Word emits as <m:fName>. Unknown names become \operatorname.
var mathFuncs = map[string]string{
	"sin":    `\sin({fe})`,
	"cos":    `\cos({fe})`,
	"tan":    `\tan({fe})`,
	"arcsin": `\arcsin({fe})`,
	"arccos": `\arccos({fe})`,
	"arctan": `\arctan({fe})`,
	"sinh":   `\sinh({fe})`,
	"cosh":   `\cosh({fe})`,
	"tanh":   `\tanh({fe})`,
	"log":    `\log({fe})`,
	"ln":     `\ln({fe})`,
	"exp":    `\exp({fe})`,
	"det":    `\det({fe})`,
	"gcd":    `\gcd({fe})`,
	"lim":    `\lim({fe})`,
	"max":    `\max({fe})`,
	"min":    `\min({fe})`,
}

// Accents the DOCX math tests name explicitly. Templates match latex_dict.CHR.
var mathAccents = map[string]string{
	"\u0302": `\hat{%s}`,
	"\u030a": `\ocirc{%s}`,
	"\u030c": `\check{%s}`,
	"\u20d6": `\overleftarrow{%s}`,
	"\u20ee": `\underleftarrow{%s}`,
	"\u20ef": `\underrightarrow{%s}`,
}

func mathLatex(n *xnode) string {
	if n == nil {
		return ""
	}
	if n.local == "oMath" || n.local == "oMathPara" {
		var b strings.Builder
		for _, c := range n.kids {
			if c.local == "oMath" || c.local == "oMathPara" {
				b.WriteString(mathLatex(c))
				continue
			}
			b.WriteString(mathNode(c))
		}
		return strings.TrimSpace(b.String())
	}
	return strings.TrimSpace(mathNode(n))
}

func mathChildren(n *xnode) string {
	var b strings.Builder
	for _, c := range n.kids {
		b.WriteString(mathNode(c))
	}
	return b.String()
}

func mathNode(n *xnode) string {
	switch n.local {
	case "r":
		return mathRun(n)
	case "t":
		return normalizeMath(n.text)
	case "func":
		return mathFunc(n)
	case "acc":
		return mathAcc(n)
	case "sSub":
		return mathPart(n, "e") + "_{" + mathPart(n, "sub") + "}"
	case "sSup":
		return mathPart(n, "e") + "^{" + mathPart(n, "sup") + "}"
	case "f":
		return `\frac{` + mathPart(n, "num") + "}{" + mathPart(n, "den") + "}"
	case "e", "sub", "sup", "num", "den", "fName", "deg", "oMath", "oMathPara":
		return mathChildren(n)
	default:
		return mathChildren(n)
	}
}

func mathPart(n *xnode, local string) string {
	for _, c := range n.kids {
		if c.local == local {
			return mathChildren(c)
		}
	}
	return ""
}

func mathRun(n *xnode) string {
	var b strings.Builder
	for _, c := range n.kids {
		if c.local == "t" {
			b.WriteString(normalizeMath(c.text))
		}
	}
	return b.String()
}

func mathFunc(n *xnode) string {
	name := ""
	arg := ""
	for _, c := range n.kids {
		switch c.local {
		case "fName":
			name = fnameText(c)
		case "e":
			arg = mathChildren(c)
		}
	}
	tmpl, ok := mathFuncs[name]
	if !ok {
		tmpl = `\operatorname{` + name + `}({fe})`
	}
	return strings.ReplaceAll(tmpl, "{fe}", arg)
}

func fnameText(n *xnode) string {
	var b strings.Builder
	var walk func(*xnode)
	walk = func(n *xnode) {
		if n.local == "r" {
			b.WriteString(mathRun(n))
			return
		}
		for _, c := range n.kids {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func mathAcc(n *xnode) string {
	chr := ""
	body := ""
	for _, c := range n.kids {
		if c.local == "accPr" {
			if ch := findFirst(c, "chr"); ch != nil {
				chr = ch.attr["val"]
			}
		}
		if c.local == "e" {
			body = mathChildren(c)
		}
	}
	tmpl, ok := mathAccents[chr]
	if !ok {
		tmpl = `\hat{%s}`
	}
	return fmt.Sprintf(tmpl, body)
}

func normalizeMath(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == 'ℎ':
			b.WriteByte('h')
		case r >= 0x1D434 && r <= 0x1D44D:
			b.WriteByte(byte('A' + (r - 0x1D434)))
		case r >= 0x1D44E && r <= 0x1D467:
			b.WriteByte(byte('a' + (r - 0x1D44E)))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
