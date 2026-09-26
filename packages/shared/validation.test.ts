import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { MESSAGES, hasErrors, isValidEmail, validateRegistration } from "./validation";

const validInput = {
  email: "ada@example.com",
  password: "correct-horse",
  confirmPassword: "correct-horse",
  displayName: "Ada",
  acceptedLegal: true,
};

describe("validateRegistration", () => {
  it("accepts a complete valid input", () => {
    assert.deepEqual(validateRegistration(validInput), {});
  });

  it("rejects a password shorter than the shared minimum", () => {
    const errors = validateRegistration({ ...validInput, password: "short", confirmPassword: "short" });
    assert.equal(errors.password, MESSAGES.passwordTooShort);
  });

  it("rejects mismatched password confirmation", () => {
    const errors = validateRegistration({ ...validInput, confirmPassword: "different" });
    assert.equal(errors.confirmPassword, MESSAGES.passwordMismatch);
  });

  // This is the case that used to pass on mobile and fail on web.
  it("rejects a one-character display name", () => {
    const errors = validateRegistration({ ...validInput, displayName: "A" });
    assert.equal(errors.displayName, MESSAGES.displayNameTooShort);
  });

  it("treats a whitespace-only display name as missing", () => {
    const errors = validateRegistration({ ...validInput, displayName: "   " });
    assert.equal(errors.displayName, MESSAGES.displayNameRequired);
  });

  it("skips the confirmation check when it is not collected", () => {
    const { confirmPassword: _omitted, ...withoutConfirmation } = validInput;
    assert.equal(validateRegistration(withoutConfirmation).confirmPassword, undefined);
  });

  it("requires the legal checkbox", () => {
    assert.equal(validateRegistration({ ...validInput, acceptedLegal: false }).legal, MESSAGES.legalRequired);
  });

  it("checks the emailed code only when the caller collects one", () => {
    assert.equal(validateRegistration({ ...validInput, code: "12345" }).code, MESSAGES.codeInvalid);
    assert.equal(validateRegistration({ ...validInput, code: "123456" }).code, undefined);
  });

  it("reports every problem at once, not just the first", () => {
    const errors = validateRegistration({
      email: "nope",
      password: "x",
      confirmPassword: "y",
      displayName: "",
      acceptedLegal: false,
    });
    assert.ok(errors.email && errors.password && errors.confirmPassword && errors.displayName && errors.legal);
    assert.equal(hasErrors(errors), true);
  });
});

describe("isValidEmail", () => {
  it("accepts ordinary addresses and trims surrounding space", () => {
    assert.equal(isValidEmail(" ada@example.com "), true);
    assert.equal(isValidEmail("ada.lovelace@sub.example.co.uk"), true);
  });

  it("rejects malformed addresses", () => {
    assert.equal(isValidEmail("ada@example"), false);
    assert.equal(isValidEmail("ada example.com"), false);
    assert.equal(isValidEmail("@example.com"), false);
  });
});
