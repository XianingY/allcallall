package user

import (
	"errors"
	"unicode"
)

// PasswordError 密码相关错误
var (
	ErrPasswordTooShort  = errors.New("password must be at least 8 characters")
	ErrPasswordTooLong   = errors.New("password must be at most 128 characters")
	ErrPasswordWeak      = errors.New("password must contain both letters and numbers")
	ErrPasswordMismatch  = errors.New("new password and confirm password do not match")
	ErrPasswordUnchanged = errors.New("new password must be different from old password")
	ErrSpecialCharacters = errors.New("password cannot contain special characters")
)

// ValidatePasswordStrength 验证密码强度
// 要求: 长度 8-128, 包含字母和数字, 不能包含特殊字符
func ValidatePasswordStrength(password string) error {
	if len(password) < 8 {
		return ErrPasswordTooShort
	}
	if len(password) > 128 {
		return ErrPasswordTooLong
	}

	hasLetter := false
	hasDigit := false

	for _, r := range password {
		if unicode.IsLetter(r) {
			hasLetter = true
		} else if unicode.IsDigit(r) {
			hasDigit = true
		} else {
			// 不允许任何其他字符（包括空格、特殊字符）
			return ErrSpecialCharacters
		}
	}

	if !hasLetter || !hasDigit {
		return ErrPasswordWeak
	}

	return nil
}

// ValidatePasswordsMatch 验证两个密码是否匹配
func ValidatePasswordsMatch(new, confirm string) error {
	if new != confirm {
		return ErrPasswordMismatch
	}
	return nil
}

// ValidatePasswordChange 验证密码修改的有效性
func ValidatePasswordChange(oldPassword, newPassword, confirmPassword string) error {
	// 验证新密码强度
	if err := ValidatePasswordStrength(newPassword); err != nil {
		return err
	}

	// 验证两次新密码是否匹配
	if err := ValidatePasswordsMatch(newPassword, confirmPassword); err != nil {
		return err
	}

	// 验证新密码与旧密码不同
	if oldPassword == newPassword {
		return ErrPasswordUnchanged
	}

	return nil
}
