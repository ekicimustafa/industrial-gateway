package env

import (
	"testing"
	"time"
)

func TestInt(t *testing.T) {
	tests := []struct {
		name string
		set  bool
		val  string
		want int
	}{
		{"unset", false, "", 5},
		{"valid", true, "12", 12},
		{"padded", true, " 7 ", 7},
		{"empty", true, "", 5},
		{"invalid", true, "abc", 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("TEST_INT", tt.val)
			}
			if got := Int("TEST_INT", 5); got != tt.want {
				t.Errorf("Int() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSeconds(t *testing.T) {
	tests := []struct {
		name string
		set  bool
		val  string
		want time.Duration
	}{
		{"unset", false, "", 30 * time.Second},
		{"whole", true, "5", 5 * time.Second},
		{"fraction", true, "0.5", 500 * time.Millisecond},
		{"negative", true, "-1", 30 * time.Second},
		{"invalid", true, "soon", 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("TEST_SECONDS", tt.val)
			}
			if got := Seconds("TEST_SECONDS", 30*time.Second); got != tt.want {
				t.Errorf("Seconds() = %v, want %v", got, tt.want)
			}
		})
	}
}
