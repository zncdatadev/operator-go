package util

import (
	"regexp"
	"strings"
)

var reTab = regexp.MustCompile(`^\t+`)

func IndentTabToSpaces(code string, spaces int) string {
	indentation := strings.Repeat(" ", spaces)
	return reTab.ReplaceAllString(code, indentation)
}

func IndentTab4Spaces(code string) string {
	return IndentTabToSpaces(code, 4)
}

func IndentTab2Spaces(code string) string {
	return IndentTabToSpaces(code, 2)
}

var re2Spaces = regexp.MustCompile(`^` + strings.Repeat(" ", 2))
var re4Spaces = regexp.MustCompile(`^` + strings.Repeat(" ", 4))

func IndentSpacesToTab(code string, spaces int) string {
	switch spaces {
	case 2:
		return re2Spaces.ReplaceAllString(code, "\t")
	case 4:
		return re4Spaces.ReplaceAllString(code, "\t")
	default:
		re := regexp.MustCompile(`^` + strings.Repeat(" ", spaces))
		return re.ReplaceAllString(code, "\t")
	}
}

func Indent4SpacesToTab(code string) string {
	return IndentSpacesToTab(code, 4)
}

func Indent2SpacesToTab(code string) string {
	return IndentSpacesToTab(code, 2)
}
