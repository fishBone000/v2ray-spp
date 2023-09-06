//go:build !confonly
// +build !confonly

package spp

import (
	"github.com/v2fly/v2ray-core/v5/common/protocol"
)

// MemoryAccount is an in-memory form of spp account.
type MemoryAccount struct {
	User     string
	Password string
}

// Equals implements protocol.Account.
func (a *MemoryAccount) Equals(account protocol.Account) bool {
	sppAccount, ok := account.(*MemoryAccount)
	if !ok {
		return false
	}
	return a.User == sppAccount.User && a.Password == sppAccount.Password
}

// AsAccount implements protocol.AsAccount.
func (a *Account) AsAccount() (protocol.Account, error) {
	if a.User == "" {
		return nil, newError("user cannot be empty").AtError()
	}
	if a.Password == "" {
		return nil, newError("password cannot be empty").AtError()
	}
	return &MemoryAccount{
		User:     a.User,
		Password: a.Password,
	}, nil
}
