import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  beginPasskeyLogin: vi.fn(),
  finishPasskeyLogin: vi.fn(),
  beginRegistration: vi.fn(),
  finishRegistration: vi.fn(),
}));
vi.mock("@/api/APIClient", () => ({
  APIClient: {
    auth: {
      beginPasskeyLogin: api.beginPasskeyLogin,
      finishPasskeyLogin: api.finishPasskeyLogin,
    },
    passkeys: {
      beginRegistration: api.beginRegistration,
      finishRegistration: api.finishRegistration,
    },
  },
}));

import { PasskeyPromptError } from "./passkeyErrors";
import { addPasskey, passkeysAvailable, signInWithPasskey } from "./passkeys";

const credential = (response: object) =>
  ({
    id: "AQID",
    rawId: new Uint8Array([1, 2, 3]).buffer,
    type: "public-key",
    authenticatorAttachment: null,
    response,
    getClientExtensionResults: () => ({}),
  }) as unknown as PublicKeyCredential;

const requestJSON = { publicKey: { challenge: "BAUG" } };
const creationJSON = {
  publicKey: {
    rp: { name: "SyncYomi" },
    user: { id: "AQID", name: "admin", displayName: "admin" },
    challenge: "BAUG",
    pubKeyCredParams: [{ type: "public-key", alg: -7 }],
  },
};

describe("passkeys in the browser", () => {
  const get = vi.fn();
  const create = vi.fn();

  beforeEach(() => {
    Object.values(api).forEach((mock) => mock.mockReset());
    get.mockReset();
    create.mockReset();
    vi.stubGlobal("navigator", { credentials: { get, create } });
    vi.stubGlobal("window", { isSecureContext: true });
    vi.stubGlobal("PublicKeyCredential", class {});
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  describe("passkeysAvailable", () => {
    it("is true in a secure context that supports WebAuthn", () => {
      expect(passkeysAvailable()).toBe(true);
    });

    it("is false on an insecure page such as http over an IP address", () => {
      vi.stubGlobal("window", { isSecureContext: false });

      expect(passkeysAvailable()).toBe(false);
    });

    it("is false when the browser has no WebAuthn", () => {
      vi.stubGlobal("PublicKeyCredential", undefined);

      expect(passkeysAvailable()).toBe(false);
    });
  });

  describe("signInWithPasskey", () => {
    it("fetches options, asks the authenticator and sends the assertion back", async () => {
      api.beginPasskeyLogin.mockResolvedValue(requestJSON);
      get.mockResolvedValue(
        credential({
          clientDataJSON: new Uint8Array([1]).buffer,
          authenticatorData: new Uint8Array([2]).buffer,
          signature: new Uint8Array([3]).buffer,
          userHandle: new Uint8Array([4]).buffer,
        }),
      );
      api.finishPasskeyLogin.mockResolvedValue({ username: "admin" });

      const username = await signInWithPasskey();

      expect(username).toBe("admin");
      expect(get.mock.calls[0][0].publicKey.challenge).toBeInstanceOf(
        ArrayBuffer,
      );
      expect(api.finishPasskeyLogin.mock.calls[0][0]).toMatchObject({
        id: "AQID",
        response: { clientDataJSON: "AQ", signature: "Aw", userHandle: "BA" },
      });
    });

    it.each(["NotAllowedError", "AbortError"])(
      "turns %s from the prompt into a friendly error and does not call the server",
      async (name) => {
        api.beginPasskeyLogin.mockResolvedValue(requestJSON);
        get.mockRejectedValue(new DOMException("cancelled", name));

        await expect(signInWithPasskey()).rejects.toThrow(PasskeyPromptError);
        expect(api.finishPasskeyLogin).not.toHaveBeenCalled();
      },
    );

    it("fails clearly when the browser returns no credential", async () => {
      api.beginPasskeyLogin.mockResolvedValue(requestJSON);
      get.mockResolvedValue(null);

      await expect(signInWithPasskey()).rejects.toThrow(PasskeyPromptError);
    });

    it("lets server errors through untouched", async () => {
      api.beginPasskeyLogin.mockRejectedValue(
        new Error("Too many failed attempts"),
      );

      await expect(signInWithPasskey()).rejects.toThrow(
        "Too many failed attempts",
      );
    });
  });

  describe("addPasskey", () => {
    it("checks the password with the server, asks the authenticator and stores the result", async () => {
      api.beginRegistration.mockResolvedValue(creationJSON);
      create.mockResolvedValue(
        credential({
          clientDataJSON: new Uint8Array([1]).buffer,
          attestationObject: new Uint8Array([2]).buffer,
          getTransports: () => ["internal"],
        }),
      );
      api.finishRegistration.mockResolvedValue({ id: 7, name: "Laptop" });

      const added = await addPasskey("Laptop", "right-password-1");

      expect(api.beginRegistration).toHaveBeenCalledWith(
        "Laptop",
        "right-password-1",
      );
      expect(create.mock.calls[0][0].publicKey.user.id).toBeInstanceOf(
        ArrayBuffer,
      );
      expect(api.finishRegistration.mock.calls[0][0]).toMatchObject({
        response: { attestationObject: "Ag", transports: ["internal"] },
      });
      expect(added).toEqual({ id: 7, name: "Laptop" });
    });

    it("does not touch the authenticator when the password is wrong", async () => {
      api.beginRegistration.mockRejectedValue(
        new Error("Current password is incorrect"),
      );

      await expect(addPasskey("Laptop", "nope")).rejects.toThrow(
        "Current password is incorrect",
      );
      expect(create).not.toHaveBeenCalled();
    });

    it("explains a cancelled prompt", async () => {
      api.beginRegistration.mockResolvedValue(creationJSON);
      create.mockRejectedValue(
        new DOMException("cancelled", "NotAllowedError"),
      );

      await expect(addPasskey("Laptop", "right-password-1")).rejects.toThrow(
        PasskeyPromptError,
      );
      expect(api.finishRegistration).not.toHaveBeenCalled();
    });

    it("explains that this authenticator already has a passkey", async () => {
      api.beginRegistration.mockResolvedValue(creationJSON);
      create.mockRejectedValue(new DOMException("exists", "InvalidStateError"));

      await expect(addPasskey("Laptop", "right-password-1")).rejects.toThrow(
        /already/i,
      );
    });
  });
});
