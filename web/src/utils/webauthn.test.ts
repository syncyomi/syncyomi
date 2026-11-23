import { describe, expect, it } from "vitest";
import {
  assertionToJSON,
  attestationToJSON,
  base64urlToBuffer,
  bufferToBase64url,
  creationOptionsFromJSON,
  requestOptionsFromJSON,
} from "./webauthn";

const bytes = (...values: number[]) => new Uint8Array(values).buffer;
const asArray = (buffer: BufferSource | ArrayBuffer) =>
  Array.from(new Uint8Array(buffer as ArrayBuffer));

describe("base64url helpers", () => {
  it("round-trips bytes that need the url-safe alphabet and no padding", () => {
    const original = bytes(251, 255, 190, 0, 1, 2, 3);

    const encoded = bufferToBase64url(original);

    expect(encoded).toBe("-_--AAECAw");
    expect(asArray(base64urlToBuffer(encoded))).toEqual(asArray(original));
  });

  it("decodes values with and without padding", () => {
    expect(asArray(base64urlToBuffer("AQI"))).toEqual([1, 2]);
    expect(asArray(base64urlToBuffer("AQI="))).toEqual([1, 2]);
  });

  it("handles an empty buffer", () => {
    expect(bufferToBase64url(new ArrayBuffer(0))).toBe("");
    expect(asArray(base64urlToBuffer(""))).toEqual([]);
  });
});

describe("creationOptionsFromJSON", () => {
  it("decodes the challenge, user id and excluded credential ids, keeping the rest", () => {
    const options = creationOptionsFromJSON({
      publicKey: {
        rp: { id: "sync.example.com", name: "SyncYomi" },
        user: { id: "AQID", name: "admin", displayName: "admin" },
        challenge: "BAUG",
        pubKeyCredParams: [{ type: "public-key", alg: -7 }],
        timeout: 60000,
        excludeCredentials: [
          { id: "BwgJ", type: "public-key", transports: ["internal"] },
        ],
        authenticatorSelection: {
          residentKey: "required",
          userVerification: "required",
        },
        attestation: "none",
      },
    });

    expect(asArray(options.challenge)).toEqual([4, 5, 6]);
    expect(asArray(options.user.id)).toEqual([1, 2, 3]);
    expect(options.user.name).toBe("admin");
    expect(options.rp.id).toBe("sync.example.com");
    expect(asArray(options.excludeCredentials![0].id)).toEqual([7, 8, 9]);
    expect(options.excludeCredentials![0].transports).toEqual(["internal"]);
    expect(options.authenticatorSelection?.residentKey).toBe("required");
    expect(options.attestation).toBe("none");
  });

  it("copes with no excluded credentials", () => {
    const options = creationOptionsFromJSON({
      publicKey: {
        rp: { name: "SyncYomi" },
        user: { id: "AQID", name: "a", displayName: "a" },
        challenge: "BAUG",
        pubKeyCredParams: [],
      },
    });

    expect(options.excludeCredentials ?? []).toEqual([]);
  });
});

describe("requestOptionsFromJSON", () => {
  it("decodes the challenge and allowed credentials", () => {
    const options = requestOptionsFromJSON({
      publicKey: {
        challenge: "BAUG",
        rpId: "sync.example.com",
        userVerification: "required",
        allowCredentials: [{ id: "BwgJ", type: "public-key" }],
      },
    });

    expect(asArray(options.challenge)).toEqual([4, 5, 6]);
    expect(options.rpId).toBe("sync.example.com");
    expect(options.userVerification).toBe("required");
    expect(asArray(options.allowCredentials![0].id)).toEqual([7, 8, 9]);
  });

  it("allows discoverable logins with no allowed credentials", () => {
    const options = requestOptionsFromJSON({
      publicKey: { challenge: "BAUG" },
    });

    expect(options.allowCredentials ?? []).toEqual([]);
  });
});

describe("credential serialisation", () => {
  const credentialWith = (response: object) =>
    ({
      id: "AQID",
      rawId: bytes(1, 2, 3),
      type: "public-key",
      authenticatorAttachment: "platform",
      response,
      getClientExtensionResults: () => ({}),
    }) as unknown as PublicKeyCredential;

  it("encodes an attestation for the server", () => {
    const json = attestationToJSON(
      credentialWith({
        clientDataJSON: bytes(10, 11),
        attestationObject: bytes(12, 13),
        getTransports: () => ["internal", "hybrid"],
      }),
    );

    expect(json).toEqual({
      id: "AQID",
      rawId: "AQID",
      type: "public-key",
      authenticatorAttachment: "platform",
      clientExtensionResults: {},
      response: {
        clientDataJSON: "Cgs",
        attestationObject: "DA0",
        transports: ["internal", "hybrid"],
      },
    });
  });

  it("encodes an attestation from an authenticator that cannot list transports", () => {
    const json = attestationToJSON(
      credentialWith({
        clientDataJSON: bytes(10),
        attestationObject: bytes(12),
      }),
    ) as { response: { transports: string[] } };

    expect(json.response.transports).toEqual([]);
  });

  it("encodes an assertion for the server, including the user handle", () => {
    const json = assertionToJSON(
      credentialWith({
        clientDataJSON: bytes(10, 11),
        authenticatorData: bytes(14),
        signature: bytes(15, 16),
        userHandle: bytes(17),
      }),
    );

    expect(json).toEqual({
      id: "AQID",
      rawId: "AQID",
      type: "public-key",
      authenticatorAttachment: "platform",
      clientExtensionResults: {},
      response: {
        clientDataJSON: "Cgs",
        authenticatorData: "Dg",
        signature: "DxA",
        userHandle: "EQ",
      },
    });
  });

  it("sends a null user handle when the authenticator returns none", () => {
    const json = assertionToJSON(
      credentialWith({
        clientDataJSON: bytes(10),
        authenticatorData: bytes(14),
        signature: bytes(15),
        userHandle: null,
      }),
    ) as { response: { userHandle: string | null } };

    expect(json.response.userHandle).toBeNull();
  });
});
