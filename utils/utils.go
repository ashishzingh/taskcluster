package utils

import (
	"strings"
)

// indents a block of text with an indent string, see http://play.golang.org/p/nV1_VLau7C
func Indent(text, indent string) string {
	if text == "" {
		return text
	}
	if text[len(text)-1:] == "\n" {
		result := ""
		for _, j := range strings.Split(text[:len(text)-1], "\n") {
			result += indent + j + "\n"
		}
		return result
	}
	result := ""
	for _, j := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		result += indent + j + "\n"
	}
	return result[:len(result)-1]
}

func Underline(text string) string {
	return text + "\n" + strings.Repeat("=", len(text)) + "\n"
}

func ExitOnFail(err error) {
	if err != nil {
		panic(err)
	}
}
