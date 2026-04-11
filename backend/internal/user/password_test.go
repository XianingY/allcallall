package user

import (
	"errors"
	"strings"
	"testing"
)

func TestValidatePasswordStrength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{name: "too short", password: "Abc1234", wantErr: ErrPasswordTooShort},
		{name: "too long", password: strings.Repeat("Ab1", 43), wantErr: ErrPasswordTooLong},
		{name: "letters only", password: "abcdefgh", wantErr: ErrPasswordWeak},
		{name: "digits only", password: "12345678", wantErr: ErrPasswordWeak},
		{name: "special chars", password: "Abcd1234!", wantErr: ErrSpecialCharacters},
		{name: "valid", password: "Abcd1234", wantErr: nil},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidatePasswordStrength(tc.password)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("unexpected error: got %v want %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidatePasswordsMatch(t *testing.T) {
	t.Parallel()

	if err := ValidatePasswordsMatch("abc", "abc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ValidatePasswordsMatch("abc", "def"); !errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("expected mismatch error, got %v", err)
	}
}

func TestValidatePasswordChange(t *testing.T) {
	t.Parallel()

	if err := ValidatePasswordChange("Oldpass1", "Newpass1", "Newpass1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ValidatePasswordChange("Oldpass1", "Newpass1", "Mismatch1"); !errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("expected mismatch error, got %v", err)
	}
	if err := ValidatePasswordChange("Oldpass1", "Oldpass1", "Oldpass1"); !errors.Is(err, ErrPasswordUnchanged) {
		t.Fatalf("expected unchanged error, got %v", err)
	}
}
