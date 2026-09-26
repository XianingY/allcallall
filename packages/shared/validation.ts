/**
 * Signup rules shared by the web and mobile clients.
 *
 * Both clients used to check signup input independently and had drifted: web
 * required a password confirmation and a display name of at least 2
 * characters, mobile required neither, so the same weak input was rejected on
 * one client and accepted on the other.
 *
 * Deliberately plain functions and constants rather than a zod schema. A
 * shared zod schema ran into cross-package type friction three times - a
 * generic wrapper tripped "type instantiation is excessively deep", an `as
 * const` refinement payload was rejected as readonly, and the resulting
 * inferred type no longer matched what react-hook-form's resolver expects.
 * The rules themselves matter more than the validation library, and this way
 * both clients can use them without agreeing on one.
 *
 * Web still uses zod for its form wiring; it imports the constants below so
 * the numbers and messages cannot diverge.
 *
 * Messages are in Chinese to match the UI. To localise later, return stable
 * codes and map them per client; do not write a second copy of the rules.
 */

export const PASSWORD_MIN_LENGTH = 8;
export const DISPLAY_NAME_MIN_LENGTH = 2;
export const EMAIL_CODE_LENGTH = 6;

export const MESSAGES = {
  emailInvalid: "请输入有效邮箱",
  passwordRequired: "请输入密码",
  passwordTooShort: `密码至少需要 ${PASSWORD_MIN_LENGTH} 个字符`,
  passwordMismatch: "两次输入的密码不一致",
  displayNameRequired: "请输入显示名称",
  displayNameTooShort: `显示名称至少需要 ${DISPLAY_NAME_MIN_LENGTH} 个字符`,
  codeInvalid: `请输入 ${EMAIL_CODE_LENGTH} 位验证码`,
  legalRequired: "请阅读并同意条款",
} as const;

// Deliberately permissive: one @, something either side, no whitespace.
// Stricter patterns reject valid addresses at signup, which is a worse
// failure than letting a typo through to the verification email.
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function isValidEmail(email: string): boolean {
  return EMAIL_PATTERN.test(email.trim());
}

export function isValidEmailCode(code: string): boolean {
  return new RegExp(`^\\d{${EMAIL_CODE_LENGTH}}$`).test(code.trim());
}

export type RegistrationField = "displayName" | "email" | "password" | "confirmPassword" | "code" | "legal";

export type RegistrationErrors = Partial<Record<RegistrationField, string>>;

export interface RegistrationInput {
  email: string;
  password: string;
  /** Omit to skip the confirmation check. */
  confirmPassword?: string;
  displayName: string;
  acceptedLegal: boolean;
  /** Omit to skip the code check. */
  code?: string;
}

/**
 * Returns one message per invalid field. An empty object means the input is
 * acceptable, so callers can render nothing.
 */
export function validateRegistration(input: RegistrationInput): RegistrationErrors {
  const errors: RegistrationErrors = {};

  if (!isValidEmail(input.email)) {
    errors.email = MESSAGES.emailInvalid;
  }

  if (!input.password) {
    errors.password = MESSAGES.passwordRequired;
  } else if (input.password.length < PASSWORD_MIN_LENGTH) {
    errors.password = MESSAGES.passwordTooShort;
  }

  // Only enforced when the caller collects a confirmation, so the mobile
  // two-step flow can opt in without the server contract changing.
  if (input.confirmPassword !== undefined && input.confirmPassword !== input.password) {
    errors.confirmPassword = MESSAGES.passwordMismatch;
  }

  const displayName = input.displayName.trim();
  if (!displayName) {
    errors.displayName = MESSAGES.displayNameRequired;
  } else if (displayName.length < DISPLAY_NAME_MIN_LENGTH) {
    errors.displayName = MESSAGES.displayNameTooShort;
  }

  if (input.code !== undefined && !isValidEmailCode(input.code)) {
    errors.code = MESSAGES.codeInvalid;
  }

  if (!input.acceptedLegal) {
    errors.legal = MESSAGES.legalRequired;
  }

  return errors;
}

export function hasErrors(errors: RegistrationErrors): boolean {
  return Object.keys(errors).length > 0;
}
