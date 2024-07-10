package util

import (
	"regexp"
	"strconv"
	"strings"
)

var reTab = regexp.MustCompile(`(^|\n)\t+`)

// IndentTabToSpaces converts leading tabs in a string to a specified number of spaces per tab.
func IndentTabToSpaces(code string, spacesPerTab int) string {
	// Use ReplaceAllStringFunc to replace each match with a dynamic replacement.
	return reTab.ReplaceAllStringFunc(code, func(match string) string {
		// Calculate the indentation by multiplying the number of tabs by spacesPerTab.
		// Adjust the length calculation if the match includes a newline.
		indentation := strings.Repeat(" ", (len(match))*spacesPerTab)
		// Check if the match includes a newline at the beginning.
		startsWithNewLine := strings.HasPrefix(match, "\n")

		if startsWithNewLine {
			// If the match started with a newline, prepend it to the indentation.
			indentation = "\n" + strings.Repeat(" ", (len(match)-len("\n"))*spacesPerTab)
		}
		return indentation
	})
}

func IndentTab4Spaces(code string) string {
	return IndentTabToSpaces(code, 4)
}

func IndentTab2Spaces(code string) string {
	return IndentTabToSpaces(code, 2)
}

// Precompiled regular expressions for 2 and 4 spaces, to improve performance.
var (
	reTwoSpaces  = regexp.MustCompile(`(^|\n)( {2})+`)
	reFourSpaces = regexp.MustCompile(`(^|\n)( {4})+`)
)

// IndentSpacesToTab converts leading spaces in a string to tabs, optimized for 2 or 4 spaces per tab.
func IndentSpacesToTab(code string, spacesPerTab int) string {
	var re *regexp.Regexp

	// Select the precompiled regular expression based on spacesPerTab.
	switch spacesPerTab {
	case 2:
		re = reTwoSpaces
	case 4:
		re = reFourSpaces
	default:
		// Dynamically compile a regular expression for other numbers of spaces per tab.
		re = regexp.MustCompile(`(^|\n)( {` + strconv.Itoa(spacesPerTab) + `})+`)
	}

	return re.ReplaceAllStringFunc(code, func(match string) string {
		// Calculate the number of tabs to replace based on the length of the match divided by spacesPerTab.
		// Adjust for potential newline character in the match.
		newLinePrefix := strings.HasPrefix(match, "\n")
		if newLinePrefix {
			match = match[1:] // Remove the newline character for calculation.
		}

		spaceCount := len(match)
		tabCount := spaceCount / spacesPerTab
		replacement := strings.Repeat("\t", tabCount)

		if newLinePrefix {
			replacement = "\n" + replacement
		}
		return replacement
	})
}

func Indent4SpacesToTab(code string) string {
	return IndentSpacesToTab(code, 4)
}

func Indent2SpacesToTab(code string) string {
	return IndentSpacesToTab(code, 2)
}
