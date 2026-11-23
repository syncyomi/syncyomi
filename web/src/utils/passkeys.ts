import { APIClient } from "@/api/APIClient";
import { PasskeyPromptError } from "@/utils/passkeyErrors";
import {
  assertionToJSON,
  attestationToJSON,
  creationOptionsFromJSON,
  requestOptionsFromJSON,
} from "@/utils/webauthn";

const TEXT = {
  cancelled: "The passkey prompt was cancelled or timed out.",
  noCredential: "The browser did not return a passkey.",
  alreadyRegistered: "This device already has a passkey for SyncYomi.",
};

export function passkeysAvailable(): boolean {
  return (
    window.isSecureContext === true &&
    typeof PublicKeyCredential !== "undefined"
  );
}

const isPublicKeyCredential = (
  credential: Credential | null,
): credential is PublicKeyCredential =>
  credential !== null &&
  credential.type === "public-key" &&
  "response" in credential;

async function prompt(
  ask: () => Promise<Credential | null>,
): Promise<PublicKeyCredential> {
  try {
    const result = await ask();
    if (!isPublicKeyCredential(result))
      throw new PasskeyPromptError(TEXT.noCredential);

    return result;
  } catch (error) {
    if (error instanceof PasskeyPromptError) throw error;
    if (error instanceof DOMException) {
      if (error.name === "InvalidStateError")
        throw new PasskeyPromptError(TEXT.alreadyRegistered);
      if (error.name === "NotAllowedError" || error.name === "AbortError")
        throw new PasskeyPromptError(TEXT.cancelled);
    }

    throw error;
  }
}

export async function signInWithPasskey(): Promise<string> {
  const options = await APIClient.auth.beginPasskeyLogin();
  const credential = await prompt(() =>
    navigator.credentials.get({
      publicKey: requestOptionsFromJSON(options),
    }),
  );

  const { username } = await APIClient.auth.finishPasskeyLogin(
    assertionToJSON(credential),
  );

  return username;
}

export async function addPasskey(
  name: string,
  currentPassword: string,
): Promise<Passkey> {
  const options = await APIClient.passkeys.beginRegistration(
    name,
    currentPassword,
  );
  const credential = await prompt(() =>
    navigator.credentials.create({
      publicKey: creationOptionsFromJSON(options),
    }),
  );

  return APIClient.passkeys.finishRegistration(attestationToJSON(credential));
}
