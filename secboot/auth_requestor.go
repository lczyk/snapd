// -*- Mode: Go; indent-tabs-mode: t -*-
//go:build !nosecboot

package secboot

import (
	"context"
	"errors"

	sb "github.com/snapcore/secboot"
)

type systemdAuthRequestor struct{}

func (r *systemdAuthRequestor) RequestUserCredential(ctx context.Context, name, path string, authTypes sb.UserAuthType) (string, sb.UserAuthType, error) {
	return "", 0, errors.New("systemd auth requestor not supported in this build")
}

func (r *systemdAuthRequestor) NotifyUserAuthResult(ctx context.Context, result sb.UserAuthResult, authTypes, exhaustedAuthTypes sb.UserAuthType) error {
	return sb.ErrAuthRequestorNotAvailable
}

func NewSystemdAuthRequestor() sb.AuthRequestor {
	return &systemdAuthRequestor{}
}
