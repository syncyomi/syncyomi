package passkey

import (
	"crypto/sha256"
	"strconv"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/go-webauthn/webauthn/webauthn"
)

type webauthnUser struct {
	user        domain.User
	credentials []webauthn.Credential
}

func userHandle(user domain.User) []byte {
	sum := sha256.Sum256([]byte("syncyomi-passkey-user:" + strconv.Itoa(user.ID)))
	return sum[:]
}

func (u webauthnUser) WebAuthnID() []byte                         { return userHandle(u.user) }
func (u webauthnUser) WebAuthnName() string                       { return u.user.Username }
func (u webauthnUser) WebAuthnDisplayName() string                { return u.user.Username }
func (u webauthnUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }
