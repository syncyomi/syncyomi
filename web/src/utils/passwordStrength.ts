import { MIN_PASSWORD_LENGTH, passwordLength } from "@/domain/passwordPolicy";

export type PasswordStrengthLabel =
  "Too short" | "Weak" | "Fair" | "Good" | "Strong";

export interface PasswordStrength {
  score: number;
  label: PasswordStrengthLabel;
}

export const MAX_STRENGTH_SCORE = 4;

const LABELS: PasswordStrengthLabel[] = [
  "Too short",
  "Weak",
  "Fair",
  "Good",
  "Strong",
];
const MIN_UNIQUE_CHARACTERS = 5;
const GOOD_LENGTH = 12;
const LONG_LENGTH = 16;
const MIXED_CLASSES = 3;

const countCharacterClasses = (password: string) =>
  [/[a-z]/, /[A-Z]/, /\d/, /[^a-zA-Z\d]/].filter((pattern) =>
    pattern.test(password),
  ).length;

export function passwordStrength(password: string): PasswordStrength {
  if (passwordLength(password) < MIN_PASSWORD_LENGTH) {
    return { score: 0, label: LABELS[0] };
  }

  if (new Set(password).size < MIN_UNIQUE_CHARACTERS) {
    return { score: 1, label: LABELS[1] };
  }

  const score = Math.min(
    MAX_STRENGTH_SCORE,
    1 +
      Number(countCharacterClasses(password) >= MIXED_CLASSES) +
      Number(passwordLength(password) >= GOOD_LENGTH) +
      Number(passwordLength(password) >= LONG_LENGTH),
  );

  return { score, label: LABELS[score] };
}
