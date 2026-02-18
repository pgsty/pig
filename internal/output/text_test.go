package output

import (
	"strings"
	"testing"
)

type nilSafeMockTexter struct {
	value string
}

func (m *nilSafeMockTexter) Text() string {
	if m == nil {
		return ""
	}
	return m.value
}

func TestResult_Text(t *testing.T) {
	tests := []struct {
		name     string
		result   *Result
		expected string
	}{
		{
			name:     "nil result",
			result:   nil,
			expected: "",
		},
		{
			name:     "success without detail",
			result:   OK("operation completed", nil),
			expected: "✓ operation completed",
		},
		{
			name:     "success with detail",
			result:   OK("installed extension", nil).WithDetail("postgis 3.4.0"),
			expected: "✓ installed extension\n  postgis 3.4.0",
		},
		{
			name:     "failure without detail",
			result:   Fail(100101, "extension not found"),
			expected: "✗ extension not found\n  Code: 100101",
		},
		{
			name:     "failure with detail",
			result:   Fail(100101, "extension not found").WithDetail("'nonexistent' is not in catalog"),
			expected: "✗ extension not found\n  'nonexistent' is not in catalog\n  Code: 100101",
		},
		{
			name:     "failure with zero code",
			result:   &Result{Success: false, Code: 0, Message: "generic error"},
			expected: "✗ generic error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.Text()
			if got != tt.expected {
				t.Errorf("Text() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestRenderDataText(t *testing.T) {
	t.Run("single texter", func(t *testing.T) {
		got := renderDataText(&mockTexter{text: "one"})
		if got != "one" {
			t.Fatalf("renderDataText(single) = %q, want %q", got, "one")
		}
	})

	t.Run("slice of texters", func(t *testing.T) {
		data := []*mockTexter{{text: "one"}, {text: "two"}}
		got := renderDataText(data)
		if got != "one\n\ntwo" {
			t.Fatalf("renderDataText(slice) = %q, want %q", got, "one\n\ntwo")
		}
	})

	t.Run("slice of pointers with nil item", func(t *testing.T) {
		data := []*nilSafeMockTexter{
			{value: "one"},
			nil,
			{value: "two"},
		}
		got := renderDataText(data)
		if got != "one\n\ntwo" {
			t.Fatalf("renderDataText(slice ptr) = %q, want %q", got, "one\n\ntwo")
		}
	})

	t.Run("non texter value", func(t *testing.T) {
		got := renderDataText([]string{"a", "b"})
		if got != "" {
			t.Fatalf("renderDataText(non-texter) = %q, want empty", got)
		}
	})
}

func TestResult_Text_EmptyFields(t *testing.T) {
	// Test with empty message
	r := &Result{Success: true, Code: 0, Message: ""}
	got := r.Text()
	if !strings.HasPrefix(got, "✓ ") {
		t.Errorf("Text() with empty message should start with '✓ ', got %q", got)
	}

	// Test with only message - detail should not appear
	r = &Result{Success: true, Code: 0, Message: "simple message"}
	got = r.Text()
	expected := "✓ simple message"
	if got != expected {
		t.Errorf("Text() = %q, want %q", got, expected)
	}
}

func TestResult_ColorText(t *testing.T) {
	// Note: ColorText falls back to Text() when not running in a TTY,
	// so we test the color logic indirectly through getColor()
	tests := []struct {
		name         string
		result       *Result
		expectColor  string
		expectSymbol string
	}{
		{
			name:         "nil result",
			result:       nil,
			expectColor:  "",
			expectSymbol: "",
		},
		{
			name:         "success uses green",
			result:       OK("done", nil),
			expectColor:  colorGreen,
			expectSymbol: "✓",
		},
		{
			name:         "error uses red",
			result:       Fail(100801, "operation failed"),
			expectColor:  colorRed,
			expectSymbol: "✗",
		},
		{
			name:         "warning state category uses yellow",
			result:       Fail(100601, "state issue"),
			expectColor:  colorYellow,
			expectSymbol: "✗",
		},
		{
			name:         "success with warning category uses yellow",
			result:       &Result{Success: true, Code: 100601, Message: "state warning"},
			expectColor:  colorYellow,
			expectSymbol: "✓",
		},
		{
			name:         "warning config category uses yellow",
			result:       Fail(100701, "config issue"),
			expectColor:  colorYellow,
			expectSymbol: "✗",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result == nil {
				got := tt.result.ColorText()
				if got != "" {
					t.Errorf("ColorText() for nil = %q, want empty", got)
				}
				return
			}

			// Test getColor() directly since TTY detection affects ColorText()
			gotColor := tt.result.getColor()
			if gotColor != tt.expectColor {
				t.Errorf("getColor() = %q, want %q", gotColor, tt.expectColor)
			}

			// Text() should contain the symbol
			got := tt.result.Text()
			if !strings.Contains(got, tt.expectSymbol) {
				t.Errorf("Text() should contain %q, got %q", tt.expectSymbol, got)
			}
		})
	}
}

func TestResult_ColorText_NoColor(t *testing.T) {
	// Test NO_COLOR environment variable
	t.Setenv("NO_COLOR", "1")

	r := OK("test", nil)
	got := r.ColorText()

	// Should not contain any ANSI escape codes
	if strings.Contains(got, "\033[") {
		t.Errorf("ColorText() with NO_COLOR set should not contain ANSI codes, got %q", got)
	}

	// Should equal plain text
	if got != r.Text() {
		t.Errorf("ColorText() with NO_COLOR = %q, want Text() = %q", got, r.Text())
	}
}

func TestResult_ColorText_DumbTerm(t *testing.T) {
	t.Setenv("TERM", "dumb")

	r := OK("test", nil)
	got := r.ColorText()

	// Should not contain any ANSI escape codes
	if strings.Contains(got, "\033[") {
		t.Errorf("ColorText() with TERM=dumb should not contain ANSI codes, got %q", got)
	}
}

func TestIsColorEnabled(t *testing.T) {
	// Note: IsColorEnabled() also checks for TTY, which will be false in tests.
	// We test the environment variable logic here.

	t.Run("NO_COLOR set disables color", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		t.Setenv("TERM", "xterm-256color")
		if IsColorEnabled() {
			t.Error("IsColorEnabled() should return false when NO_COLOR is set")
		}
	})

	t.Run("TERM dumb disables color", func(t *testing.T) {
		t.Setenv("TERM", "dumb")
		if IsColorEnabled() {
			t.Error("IsColorEnabled() should return false when TERM=dumb")
		}
	})

	// Note: Testing "default enabled" is not reliable because tests don't run in a TTY
}

func TestRenderTable(t *testing.T) {
	tests := []struct {
		name     string
		headers  []string
		rows     [][]string
		expected string
	}{
		{
			name:     "empty headers",
			headers:  []string{},
			rows:     [][]string{{"a", "b"}},
			expected: "",
		},
		{
			name:    "simple table",
			headers: []string{"NAME", "VERSION"},
			rows: [][]string{
				{"postgis", "3.4.0"},
				{"vector", "0.7.0"},
			},
			expected: "NAME     VERSION\n────────────────\npostgis  3.4.0  \nvector   0.7.0  \n",
		},
		{
			name:    "single column",
			headers: []string{"NAME"},
			rows: [][]string{
				{"postgis"},
				{"vector"},
			},
			expected: "NAME   \n───────\npostgis\nvector \n",
		},
		{
			name:     "with empty rows",
			headers:  []string{"NAME", "VALUE"},
			rows:     [][]string{},
			expected: "NAME  VALUE\n───────────\n",
		},
		{
			name:    "uneven row length",
			headers: []string{"A", "B", "C"},
			rows: [][]string{
				{"1", "2"},
				{"3", "4", "5"},
			},
			expected: "A  B  C\n───────\n1  2   \n3  4  5\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderTable(tt.headers, tt.rows)
			if got != tt.expected {
				t.Errorf("RenderTable() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestRenderTable_Unicode(t *testing.T) {
	// Test that Unicode characters are handled correctly
	headers := []string{"名称", "版本"}
	rows := [][]string{
		{"扩展", "1.0"},
	}
	got := RenderTable(headers, rows)

	// Should contain the Unicode characters
	if !strings.Contains(got, "名称") {
		t.Error("RenderTable() should contain Unicode header")
	}
	if !strings.Contains(got, "扩展") {
		t.Error("RenderTable() should contain Unicode data")
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		s        string
		width    int
		expected string
	}{
		{"abc", 5, "abc  "},
		{"abc", 3, "abc"},
		{"abc", 2, "abc"},
		{"", 3, "   "},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := padRight(tt.s, tt.width)
			if got != tt.expected {
				t.Errorf("padRight(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.expected)
			}
		})
	}
}

func TestPadRight_Unicode(t *testing.T) {
	// Test with CJK characters - each CJK char is 2 display width
	got := padRight("中文", 6) // 4 display width + 2 spaces = 6 width
	if got != "中文  " {
		t.Errorf("padRight(\"中文\", 6) = %q, want \"中文  \"", got)
	}

	// String already at width (中文 = 4 display width)
	got = padRight("中文", 4)
	if got != "中文" {
		t.Errorf("padRight(\"中文\", 4) = %q, want \"中文\"", got)
	}

	// String exceeds width
	got = padRight("中文", 2)
	if got != "中文" {
		t.Errorf("padRight(\"中文\", 2) = %q, want \"中文\"", got)
	}
}

func TestStringWidth(t *testing.T) {
	tests := []struct {
		s        string
		expected int
	}{
		{"abc", 3},
		{"中文", 4}, // CJK characters are 2 width each
		{"", 0},
		{"a中b", 4},     // 1 + 2 + 1
		{"Hello世界", 9}, // 5 + 2 + 2
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := stringWidth(tt.s)
			if got != tt.expected {
				t.Errorf("stringWidth(%q) = %d, want %d", tt.s, got, tt.expected)
			}
		})
	}
}

func TestRuneWidth(t *testing.T) {
	tests := []struct {
		r        rune
		expected int
	}{
		{'a', 1},
		{'中', 2},
		{'あ', 2}, // Hiragana
		{'ア', 2}, // Katakana
		{'한', 2}, // Hangul
		{'!', 1},
		{'　', 2}, // Fullwidth space
	}

	for _, tt := range tests {
		t.Run(string(tt.r), func(t *testing.T) {
			got := runeWidth(tt.r)
			if got != tt.expected {
				t.Errorf("runeWidth(%q) = %d, want %d", tt.r, got, tt.expected)
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	// Test with nil
	if isTerminal(nil) {
		t.Error("isTerminal(nil) should return false")
	}
}

func TestResult_Render_TextAndColorFormats(t *testing.T) {
	r := OK("test message", nil)

	// Test text format
	got, err := r.Render("text")
	if err != nil {
		t.Errorf("Render(text) error = %v", err)
	}
	if string(got) != r.Text() {
		t.Errorf("Render(text) = %q, want %q", got, r.Text())
	}

	// Test text-color format
	got, err = r.Render("text-color")
	if err != nil {
		t.Errorf("Render(text-color) error = %v", err)
	}
	if string(got) != r.ColorText() {
		t.Errorf("Render(text-color) = %q, want %q", got, r.ColorText())
	}
}

func TestResult_Text_NilReceiver(t *testing.T) {
	var r *Result
	got := r.Text()
	if got != "" {
		t.Errorf("nil Result.Text() = %q, want empty string", got)
	}
}

func TestResult_ColorText_NilReceiver(t *testing.T) {
	var r *Result
	got := r.ColorText()
	if got != "" {
		t.Errorf("nil Result.ColorText() = %q, want empty string", got)
	}
}

// mockTexter implements the Texter interface for testing
type mockTexter struct {
	text string
}

func (m *mockTexter) Text() string {
	return m.text
}

func TestResult_Text_WithTexter(t *testing.T) {
	// Test that Data.Text() is included when Data implements Texter
	data := &mockTexter{text: "detailed data output\nsecond line"}
	r := OK("summary message", data)
	got := r.Text()
	if !strings.Contains(got, "summary message") {
		t.Errorf("Text() should contain message, got %q", got)
	}
	if !strings.Contains(got, "detailed data output") {
		t.Errorf("Text() should contain data text, got %q", got)
	}
	if !strings.Contains(got, "second line") {
		t.Errorf("Text() should contain data text second line, got %q", got)
	}

	// Test that data with empty Text() doesn't add extra newline
	emptyData := &mockTexter{text: ""}
	r2 := OK("message only", emptyData)
	got2 := r2.Text()
	if got2 != "✓ message only" {
		t.Errorf("Text() with empty Texter = %q, want %q", got2, "✓ message only")
	}

	// Test with nil data - should not call Text()
	r3 := OK("no data", nil)
	got3 := r3.Text()
	if got3 != "✓ no data" {
		t.Errorf("Text() with nil data = %q, want %q", got3, "✓ no data")
	}

	// Test that non-Texter data is ignored
	r4 := OK("plain data", map[string]string{"key": "value"})
	got4 := r4.Text()
	if got4 != "✓ plain data" {
		t.Errorf("Text() with non-Texter data = %q, want %q", got4, "✓ plain data")
	}
}

func TestResult_Text_TexterWithDetail(t *testing.T) {
	// Test that data text appears before detail
	data := &mockTexter{text: "data output"}
	r := OK("message", data).WithDetail("some detail")
	got := r.Text()

	// Data text should come before detail
	dataIdx := strings.Index(got, "data output")
	detailIdx := strings.Index(got, "some detail")
	if dataIdx == -1 || detailIdx == -1 {
		t.Errorf("Text() should contain both data text and detail, got %q", got)
	}
	if dataIdx > detailIdx {
		t.Errorf("Data text should appear before detail in Text(), got %q", got)
	}
}
