package auth

import (
	"fmt"
	"net/http"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// PasskeyCredential is the stored representation of a registered passkey.
type PasskeyCredential struct {
	ID        []byte
	PublicKey []byte
	SignCount uint32
}

// PasskeyUser is the interface PasskeyService requires from the user domain.
// Implement this on your db.User type to avoid coupling this package to the DB layer.
type PasskeyUser interface {
	GetID() []byte // UUID bytes (16 bytes)
	GetUsername() string
	GetCredentials() []PasskeyCredential
}

// webAuthnUser adapts PasskeyUser to the webauthn.User interface.
type webAuthnUser struct {
	inner PasskeyUser
}

func (u webAuthnUser) WebAuthnID() []byte          { return u.inner.GetID() }
func (u webAuthnUser) WebAuthnName() string         { return u.inner.GetUsername() }
func (u webAuthnUser) WebAuthnDisplayName() string  { return u.inner.GetUsername() }
func (u webAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	creds := u.inner.GetCredentials()
	out := make([]webauthn.Credential, len(creds))
	for i, c := range creds {
		out[i] = webauthn.Credential{
			ID:        c.ID,
			PublicKey: c.PublicKey,
			Authenticator: webauthn.Authenticator{
				SignCount: c.SignCount,
			},
		}
	}
	return out
}

// PasskeyService wraps the WebAuthn library for registration and login ceremonies.
type PasskeyService struct {
	wa *webauthn.WebAuthn
}

// NewPasskeyService creates a PasskeyService.
//
//   - rpID:     the registered domain, e.g. "zeropad.dev"
//   - rpOrigin: the full origin callers will use, e.g. "https://zeropad.dev"
//   - rpName:   human-readable display name shown in the OS passkey dialog
func NewPasskeyService(rpID, rpOrigin, rpName string) (*PasskeyService, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: rpName,
		RPOrigins:     []string{rpOrigin},
	})
	if err != nil {
		return nil, fmt.Errorf("init webauthn: %w", err)
	}
	return &PasskeyService{wa: wa}, nil
}

// BeginRegistration starts the passkey registration ceremony.
// The returned options are sent to the browser to pass to startRegistration().
// Store sessionData server-side (e.g. in a short-lived cache keyed by user ID)
// and pass it to FinishRegistration when the browser responds.
func (s *PasskeyService) BeginRegistration(user PasskeyUser) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
	creation, session, err := s.wa.BeginRegistration(webAuthnUser{inner: user})
	if err != nil {
		return nil, nil, fmt.Errorf("begin registration: %w", err)
	}
	return creation, session, nil
}

// FinishRegistration validates the browser credential and returns the credential to store.
// Persist credential.ID, credential.PublicKey, and credential.Authenticator.SignCount
// in the passkeys table, associated with the user.
func (s *PasskeyService) FinishRegistration(user PasskeyUser, session *webauthn.SessionData, r *http.Request) (*webauthn.Credential, error) {
	credential, err := s.wa.FinishRegistration(webAuthnUser{inner: user}, *session, r)
	if err != nil {
		return nil, fmt.Errorf("finish registration: %w", err)
	}
	return credential, nil
}

// BeginLogin starts the passkey authentication ceremony.
// The returned assertion options are sent to the browser to pass to startAuthentication().
// Store sessionData server-side keyed by username and pass it to FinishLogin.
func (s *PasskeyService) BeginLogin(user PasskeyUser) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
	assertion, session, err := s.wa.BeginLogin(webAuthnUser{inner: user})
	if err != nil {
		return nil, nil, fmt.Errorf("begin login: %w", err)
	}
	return assertion, session, nil
}

// FinishLogin validates the signed browser assertion.
// On success update the stored sign count: credential.Authenticator.SignCount.
// Replaying an old sign count is a cloning attack — the library rejects it automatically.
func (s *PasskeyService) FinishLogin(user PasskeyUser, session *webauthn.SessionData, r *http.Request) (*webauthn.Credential, error) {
	credential, err := s.wa.FinishLogin(webAuthnUser{inner: user}, *session, r)
	if err != nil {
		return nil, fmt.Errorf("finish login: %w", err)
	}
	return credential, nil
}
