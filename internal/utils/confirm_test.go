package utils

import "testing"

// TestConfirmationAccepted guards the global T2 confirmation grammar (B38):
// y / yes in any case proceeds; everything else — including empty/EOF — aborts.
func TestConfirmationAccepted(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"y", true},
		{"Y", true},
		{"yes", true},
		{"YES", true},
		{" yes \n", true},
		{"y\n", true},
		{"", false},
		{"\n", false},
		{"no", false},
		{"n", false},
		{"yess", false},
		{"restore", false},
	}
	for _, tt := range tests {
		if got := ConfirmationAccepted(tt.input); got != tt.want {
			t.Errorf("ConfirmationAccepted(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
