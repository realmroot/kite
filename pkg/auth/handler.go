package auth

import "context"

type AuthHandler struct {
	oidc *oidcAuthenticator
}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{oidc: &oidcAuthenticator{}}
}

func (h *AuthHandler) IDTokenForSession(ctx context.Context, sessionID uint) (string, error) {
	return h.oidc.idTokenForSession(ctx, sessionID)
}
