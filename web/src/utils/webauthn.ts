export interface CreationOptionsJSON {
  publicKey: {
    rp: PublicKeyCredentialRpEntity;
    user: { id: string; name: string; displayName: string };
    challenge: string;
    pubKeyCredParams: PublicKeyCredentialParameters[];
    timeout?: number;
    excludeCredentials?: {
      id: string;
      type: PublicKeyCredentialType;
      transports?: AuthenticatorTransport[];
    }[];
    authenticatorSelection?: AuthenticatorSelectionCriteria;
    attestation?: AttestationConveyancePreference;
  };
}

export interface RequestOptionsJSON {
  publicKey: {
    challenge: string;
    timeout?: number;
    rpId?: string;
    allowCredentials?: {
      id: string;
      type: PublicKeyCredentialType;
      transports?: AuthenticatorTransport[];
    }[];
    userVerification?: UserVerificationRequirement;
  };
}

export function base64urlToBuffer(value: string): ArrayBuffer {
  const base64 = value.replace(/=+$/, "").replace(/-/g, "+").replace(/_/g, "/");
  const binary = atob(base64.padEnd(Math.ceil(base64.length / 4) * 4, "="));
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);

  return bytes.buffer;
}

export function bufferToBase64url(buffer: ArrayBuffer): string {
  let binary = "";
  for (const byte of new Uint8Array(buffer))
    binary += String.fromCharCode(byte);

  return btoa(binary)
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
}

const decodeDescriptors = (
  descriptors: {
    id: string;
    type: PublicKeyCredentialType;
    transports?: AuthenticatorTransport[];
  }[] = [],
): PublicKeyCredentialDescriptor[] =>
  descriptors.map(({ id, ...rest }) => ({
    ...rest,
    id: base64urlToBuffer(id),
  }));

export function creationOptionsFromJSON(
  json: CreationOptionsJSON,
): PublicKeyCredentialCreationOptions {
  const { user, challenge, excludeCredentials, ...rest } = json.publicKey;

  return {
    ...rest,
    challenge: base64urlToBuffer(challenge),
    user: { ...user, id: base64urlToBuffer(user.id) },
    excludeCredentials: decodeDescriptors(excludeCredentials),
  };
}

export function requestOptionsFromJSON(
  json: RequestOptionsJSON,
): PublicKeyCredentialRequestOptions {
  const { challenge, allowCredentials, ...rest } = json.publicKey;

  return {
    ...rest,
    challenge: base64urlToBuffer(challenge),
    allowCredentials: decodeDescriptors(allowCredentials),
  };
}

const credentialEnvelope = (credential: PublicKeyCredential) => ({
  id: credential.id,
  rawId: bufferToBase64url(credential.rawId),
  type: credential.type,
  authenticatorAttachment: credential.authenticatorAttachment,
  clientExtensionResults: credential.getClientExtensionResults(),
});

export function attestationToJSON(credential: PublicKeyCredential): object {
  const response = credential.response as AuthenticatorAttestationResponse;

  return {
    ...credentialEnvelope(credential),
    response: {
      clientDataJSON: bufferToBase64url(response.clientDataJSON),
      attestationObject: bufferToBase64url(response.attestationObject),
      transports: response.getTransports?.() ?? [],
    },
  };
}

export function assertionToJSON(credential: PublicKeyCredential): object {
  const response = credential.response as AuthenticatorAssertionResponse;

  return {
    ...credentialEnvelope(credential),
    response: {
      clientDataJSON: bufferToBase64url(response.clientDataJSON),
      authenticatorData: bufferToBase64url(response.authenticatorData),
      signature: bufferToBase64url(response.signature),
      userHandle: response.userHandle
        ? bufferToBase64url(response.userHandle)
        : null,
    },
  };
}
