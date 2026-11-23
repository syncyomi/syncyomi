export const MIN_PASSWORD_LENGTH = 8;
export const MAX_PASSWORD_BYTES = 1024;

export const passwordLength = (password: string) => [...password].length;

export const passwordBytes = (password: string) =>
  new TextEncoder().encode(password).length;
