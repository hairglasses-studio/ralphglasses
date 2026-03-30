package session

import (
	"testing"
)

func TestTaskSizePoints_KnownSizes(t *testing.T) {
	cases := []struct {
		size string
		want int
	}{
		{"S", 1},
		{"M", 2},
		{"L", 3},
	}
	for _, tc := range cases {
		t.Run(tc.size, func(t *testing.T) {
			got := taskSizePoints(tc.size)
			if got != tc.want {
				t.Errorf("taskSizePoints(%q) = %d, want %d", tc.size, got, tc.want)
			}
		})
	}
}

func TestTaskSizePoints_Unknown(t *testing.T) {
	got := taskSizePoints("XL")
	if got != 2 {
		t.Errorf("taskSizePoints(unknown) = %d, want 2 (default medium)", got)
	}
}

func TestTaskSizePoints_Empty(t *testing.T) {
	got := taskSizePoints("")
	if got != 2 {
		t.Errorf("taskSizePoints('') = %d, want 2 (default medium)", got)
	}
}
