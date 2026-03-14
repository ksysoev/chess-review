package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatMateIn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mateIn   *int
		expected string
	}{
		{
			name:     "nil returns dash",
			mateIn:   nil,
			expected: "-",
		},
		{
			name:     "positive mate-in-1",
			mateIn:   intPtr(1),
			expected: "M1",
		},
		{
			name:     "positive mate-in-3",
			mateIn:   intPtr(3),
			expected: "M3",
		},
		{
			name:     "zero",
			mateIn:   intPtr(0),
			expected: "M0",
		},
		{
			name:     "negative being-mated-in-2",
			mateIn:   intPtr(-2),
			expected: "-M2",
		},
		{
			name:     "negative being-mated-in-5",
			mateIn:   intPtr(-5),
			expected: "-M5",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := formatMateIn(tc.mateIn)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func intPtr(n int) *int {
	return &n
}
