package util

import "testing"

func TestIndentTabToSpaces(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		spaces   int
		expected string
	}{
		{
			name:     "Converts tabs to 4 spaces",
			code:     "\tfunc main() {}",
			spaces:   4,
			expected: "    func main() {}",
		},
		{
			name:     "Converts tabs to 2 spaces",
			code:     "\tfunc main() {}",
			spaces:   2,
			expected: "  func main() {}",
		},
		{
			name:     "Ignores spaces",
			code:     " func main() {}",
			spaces:   4,
			expected: " func main() {}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IndentTabToSpaces(tt.code, tt.spaces)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestIndentSpacesToTab(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		spaces   int
		expected string
	}{
		{
			name:     "Converts 4 spaces to tabs",
			code:     "    func main() {}",
			spaces:   4,
			expected: "\tfunc main() {}",
		},
		{
			name:     "Converts 2 spaces to tabs",
			code:     "  func main() {}",
			spaces:   2,
			expected: "\tfunc main() {}",
		},
		{
			name:     "Ignores tabs",
			code:     "\tfunc main() {}",
			spaces:   4,
			expected: "\tfunc main() {}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IndentSpacesToTab(tt.code, tt.spaces)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
