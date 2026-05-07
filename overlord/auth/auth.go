// chainsawed prototype: types-only stub of upstream overlord/auth.
// the full package wires user/device auth state into snapd's daemon
// state machine, persisted via state.State. the prototype has no
// daemon and no persistent state -- it just needs the types so that
// store/* compiles. user-bound auth flows (NewUser, RemoveUser,
// CheckMacaroon, ...) are gone. the store path runs anonymous.

package auth

import (
	"encoding/base64"
	"errors"
	"time"

	"gopkg.in/macaroon.v1"
)

// UserState represents an authenticated user.
type UserState struct {
	ID              int       `json:"id"`
	Username        string    `json:"username,omitempty"`
	Email           string    `json:"email,omitempty"`
	Macaroon        string    `json:"macaroon,omitempty"`
	Discharges      []string  `json:"discharges,omitempty"`
	StoreMacaroon   string    `json:"store-macaroon,omitempty"`
	StoreDischarges []string  `json:"store-discharges,omitempty"`
	Expiration      time.Time `json:"expiration,omitzero"`
}

// DeviceState represents the device's identity and store credentials.
type DeviceState struct {
	Brand           string `json:"brand,omitempty"`
	Model           string `json:"model,omitempty"`
	Serial          string `json:"serial,omitempty"`
	KeyID           string `json:"key-id,omitempty"`
	SessionMacaroon string `json:"session-macaroon,omitempty"`
}

// HasStoreAuth returns true if the user has store authorization.
func (u *UserState) HasStoreAuth() bool {
	if u == nil {
		return false
	}
	return u.StoreMacaroon != ""
}

// HasExpired returns true if the user's session has expired.
func (u *UserState) HasExpired() bool {
	if u.Expiration.IsZero() {
		return false
	}
	return u.Expiration.Before(time.Now())
}

// MacaroonSerialize returns a store-compatible serialised representation
// of the given macaroon.
func MacaroonSerialize(m *macaroon.Macaroon) (string, error) {
	marshalled, err := m.MarshalBinary()
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(marshalled), nil
}

// MacaroonDeserialize returns a deserialised macaroon from a given
// store-compatible serialisation.
func MacaroonDeserialize(serializedMacaroon string) (*macaroon.Macaroon, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(serializedMacaroon)
	if err != nil {
		return nil, err
	}
	var m macaroon.Macaroon
	if err := m.UnmarshalBinary(decoded); err != nil {
		return nil, err
	}
	return &m, nil
}

// ErrInvalidUser is returned when the user can not be found.
var ErrInvalidUser = errors.New("invalid user")

// CloudInfo describes the cloud the current system is running on.
// Returned via DeviceAndAuthContext.CloudInfo() when the store talks
// to the snap repository -- the prototype never sets this, but the
// type still has to exist for the interface.
type CloudInfo struct {
	Name             string `json:"name"`
	Region           string `json:"region,omitempty"`
	AvailabilityZone string `json:"availability-zone,omitempty"`
}
