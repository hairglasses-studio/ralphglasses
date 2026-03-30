package session

import (
	"testing"
)

func TestFormatFloat_Zero(t *testing.T) {
	got := formatFloat(0)
	if got != "0.00" {
		t.Errorf("formatFloat(0) = %q, want 0.00", got)
	}
}

func TestFormatFloat_Whole(t *testing.T) {
	got := formatFloat(3.0)
	if got != "3.00" {
		t.Errorf("formatFloat(3.0) = %q, want 3.00", got)
	}
}

func TestFormatFloat_Fractional(t *testing.T) {
	got := formatFloat(1.5)
	if got != "1.50" {
		t.Errorf("formatFloat(1.5) = %q, want 1.50", got)
	}
}

func TestFormatFloat_SmallFrac(t *testing.T) {
	got := formatFloat(2.05)
	if got != "2.05" {
		t.Errorf("formatFloat(2.05) = %q, want 2.05", got)
	}
}

func TestLtPad2_SingleDigit(t *testing.T) {
	got := ltPad2(5)
	if got != "05" {
		t.Errorf("ltPad2(5) = %q, want 05", got)
	}
}

func TestLtPad2_TwoDigits(t *testing.T) {
	got := ltPad2(42)
	if got != "42" {
		t.Errorf("ltPad2(42) = %q, want 42", got)
	}
}

func TestLtItoa_Zero(t *testing.T) {
	got := ltItoa(0)
	if got != "0" {
		t.Errorf("ltItoa(0) = %q, want 0", got)
	}
}

func TestLtItoa_Positive(t *testing.T) {
	got := ltItoa(123)
	if got != "123" {
		t.Errorf("ltItoa(123) = %q, want 123", got)
	}
}

func TestLtItoa_Negative(t *testing.T) {
	got := ltItoa(-7)
	if got != "-7" {
		t.Errorf("ltItoa(-7) = %q, want -7", got)
	}
}
