package passkey

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/go-webauthn/webauthn/protocol"
)

const (
	flagUserPresent  = 0x01
	flagUserVerified = 0x04
	flagAttested     = 0x40
)

type softAuthenticator struct {
	key          *ecdsa.PrivateKey
	credentialID []byte
	userHandle   []byte
	rpID         string
	origin       string
	signCount    uint32
	skipVerified bool
}

func newSoftAuthenticator(t *testing.T, rpID, origin string) *softAuthenticator {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	credentialID := make([]byte, 32)
	if _, err := rand.Read(credentialID); err != nil {
		t.Fatalf("generate credential id: %v", err)
	}

	return &softAuthenticator{key: key, credentialID: credentialID, rpID: rpID, origin: origin}
}

func encode(data []byte) string { return base64.RawURLEncoding.EncodeToString(data) }

func (a *softAuthenticator) clientData(t *testing.T, kind string, challenge []byte) []byte {
	t.Helper()

	data, err := json.Marshal(map[string]any{
		"type":        kind,
		"challenge":   encode(challenge),
		"origin":      a.origin,
		"crossOrigin": false,
	})
	if err != nil {
		t.Fatalf("client data: %v", err)
	}

	return data
}

func (a *softAuthenticator) rpIDHash() []byte {
	sum := sha256.Sum256([]byte(a.rpID))
	return sum[:]
}

func (a *softAuthenticator) authData(flags byte, withCredential bool, t *testing.T) []byte {
	t.Helper()

	if a.skipVerified {
		flags &^= flagUserVerified
	}

	data := append([]byte{}, a.rpIDHash()...)
	data = append(data, flags)
	data = binary.BigEndian.AppendUint32(data, a.signCount)

	if withCredential {
		cose, err := cbor.Marshal(map[int]any{
			1:  2,
			3:  -7,
			-1: 1,
			-2: a.key.PublicKey.X.FillBytes(make([]byte, 32)),
			-3: a.key.PublicKey.Y.FillBytes(make([]byte, 32)),
		})
		if err != nil {
			t.Fatalf("cose key: %v", err)
		}
		data = append(data, make([]byte, 16)...)
		data = binary.BigEndian.AppendUint16(data, uint16(len(a.credentialID)))
		data = append(data, a.credentialID...)
		data = append(data, cose...)
	}

	return data
}

func userHandleOf(t *testing.T, creation *protocol.CredentialCreation) []byte {
	t.Helper()

	switch id := creation.Response.User.ID.(type) {
	case protocol.URLEncodedBase64:
		return []byte(id)
	case []byte:
		return id
	default:
		t.Fatalf("unexpected user id type %T", id)
		return nil
	}
}

func (a *softAuthenticator) register(t *testing.T, creation *protocol.CredentialCreation) *http.Request {
	t.Helper()

	a.userHandle = userHandleOf(t, creation)
	clientData := a.clientData(t, "webauthn.create", creation.Response.Challenge)
	attestation, err := cbor.Marshal(map[string]any{
		"fmt":      "none",
		"attStmt":  map[string]any{},
		"authData": a.authData(flagUserPresent|flagUserVerified|flagAttested, true, t),
	})
	if err != nil {
		t.Fatalf("attestation object: %v", err)
	}

	return jsonRequest(t, map[string]any{
		"id":    encode(a.credentialID),
		"rawId": encode(a.credentialID),
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    encode(clientData),
			"attestationObject": encode(attestation),
			"transports":        []string{"internal"},
		},
	})
}

func (a *softAuthenticator) assert(t *testing.T, assertion *protocol.CredentialAssertion) *http.Request {
	t.Helper()

	clientData := a.clientData(t, "webauthn.get", assertion.Response.Challenge)
	authData := a.authData(flagUserPresent|flagUserVerified, false, t)
	clientHash := sha256.Sum256(clientData)
	signed := sha256.Sum256(append(append([]byte{}, authData...), clientHash[:]...))
	signature, err := ecdsa.SignASN1(rand.Reader, a.key, signed[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	return jsonRequest(t, map[string]any{
		"id":    encode(a.credentialID),
		"rawId": encode(a.credentialID),
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    encode(clientData),
			"authenticatorData": encode(authData),
			"signature":         encode(signature),
			"userHandle":        encode(a.userHandle),
		},
	})
}

func jsonRequest(t *testing.T, body any) *http.Request {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	return req
}
