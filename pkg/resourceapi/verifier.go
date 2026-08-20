package resourceapi

import (
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
)

const (
	proofLifetime = 5 * time.Minute
	principalKey  = "resource-principal"
)

var errProofReplay = errors.New("DPoP proof was already used")

type actor struct {
	Issuer  string `json:"iss"`
	Subject string `json:"sub"`
}

type principal struct {
	Subject     string
	Actor       actor
	ClientID    string
	Scopes      map[string]struct{}
	ScopeString string
	Groups      []string
	Token       string
	TokenID     string
}

type tokenClaims struct {
	Issuer   string          `json:"iss"`
	Subject  string          `json:"sub"`
	Audience json.RawMessage `json:"aud"`
	Expires  int64           `json:"exp"`
	IssuedAt int64           `json:"iat"`
	TokenID  string          `json:"jti"`
	ClientID string          `json:"client_id"`
	Scope    string          `json:"scope"`
	Groups   []string        `json:"groups"`
	Confirm  struct {
		Thumbprint string `json:"jkt"`
	} `json:"cnf"`
	Actor actor `json:"act"`
}

type proofClaims struct {
	HTTPMethod string `json:"htm"`
	HTTPURI    string `json:"htu"`
	IssuedAt   int64  `json:"iat"`
	TokenHash  string `json:"ath"`
	TokenID    string `json:"jti"`
}

type verifier struct {
	issuer            string
	resource          string
	authorizedClients map[string]struct{}
	algorithms        []string
	providerVerifier  *oidc.IDTokenVerifier
	now               func() time.Time
	replays           replayStore
}

type replayStore interface {
	Consume(context.Context, string, string, time.Time) error
}

type databaseReplayStore struct {
	db *gorm.DB
}

func newVerifier(ctx context.Context, issuer, resource string, clients, algorithms []string, db *gorm.DB) (*verifier, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discover resource authorization server: %w", err)
	}
	allowed := make(map[string]struct{}, len(clients))
	for _, client := range clients {
		allowed[client] = struct{}{}
	}
	return &verifier{
		issuer:            issuer,
		resource:          resource,
		authorizedClients: allowed,
		algorithms:        append([]string(nil), algorithms...),
		providerVerifier: provider.Verifier(&oidc.Config{
			SkipClientIDCheck:    true,
			SupportedSigningAlgs: append([]string(nil), algorithms...),
		}),
		now:     time.Now,
		replays: &databaseReplayStore{db: db},
	}, nil
}

func (v *verifier) verify(ctx context.Context, authorization, proof, method, target string) (*principal, error) {
	token, err := dpopAccessToken(authorization)
	if err != nil {
		return nil, err
	}
	if err := validateJWTHeader(token, "at+jwt", v.algorithms); err != nil {
		return nil, oauthError("invalid_token", err.Error(), 401)
	}
	verified, err := v.providerVerifier.Verify(ctx, token)
	if err != nil {
		return nil, oauthError("invalid_token", "access token verification failed", 401)
	}
	var claims tokenClaims
	if err := verified.Claims(&claims); err != nil {
		return nil, oauthError("invalid_token", "access token claims are invalid", 401)
	}
	if claims.Issuer != v.issuer || claims.Subject == "" || claims.TokenID == "" || claims.ClientID == "" {
		return nil, oauthError("invalid_token", "access token identity claims are invalid", 401)
	}
	if !audienceContains(claims.Audience, v.resource) {
		return nil, oauthError("invalid_token", "access token audience is invalid", 401)
	}
	if _, ok := v.authorizedClients[claims.ClientID]; !ok {
		return nil, oauthError("invalid_token", "access token client is not authorized", 401)
	}
	if claims.Actor.Subject == "" || claims.Actor.Issuer == "" {
		return nil, oauthError("invalid_token", "access token has no attributable actor", 401)
	}
	if claims.Actor.Issuer != v.issuer {
		return nil, oauthError("invalid_token", "access token actor issuer is not trusted", 401)
	}
	if claims.Confirm.Thumbprint == "" {
		return nil, oauthError("invalid_token", "access token is not proof-of-possession bound", 401)
	}

	proofClaims, thumbprint, err := verifyProof(proof, method, target, v.now())
	if err != nil {
		return nil, err
	}
	if thumbprint != claims.Confirm.Thumbprint {
		return nil, oauthError("invalid_token", "DPoP key does not match the access token", 401)
	}
	expectedHash := sha256.Sum256([]byte(token))
	if proofClaims.TokenHash != base64.RawURLEncoding.EncodeToString(expectedHash[:]) {
		return nil, oauthError("invalid_dpop_proof", "DPoP access token hash is invalid", 401)
	}
	if err := v.replays.Consume(ctx, thumbprint, proofClaims.TokenID, v.now().Add(proofLifetime)); err != nil {
		if errors.Is(err, errProofReplay) {
			return nil, oauthError("invalid_dpop_proof", err.Error(), 401)
		}
		return nil, oauthError("server_error", "DPoP replay state is unavailable", 500)
	}

	scopes := make(map[string]struct{})
	for _, scope := range strings.Fields(claims.Scope) {
		scopes[scope] = struct{}{}
	}
	return &principal{
		Subject:     claims.Subject,
		Actor:       claims.Actor,
		ClientID:    claims.ClientID,
		Scopes:      scopes,
		ScopeString: claims.Scope,
		Groups:      append([]string(nil), claims.Groups...),
		Token:       token,
		TokenID:     claims.TokenID,
	}, nil
}

func dpopAccessToken(value string) (string, error) {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "DPoP") || parts[1] == "" {
		return "", oauthError("invalid_token", "DPoP access token is required", 401)
	}
	return parts[1], nil
}

func validateJWTHeader(compact, expectedType string, algorithms []string) error {
	parts := strings.Split(compact, ".")
	if len(parts) != 3 {
		return errors.New("access token is malformed")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return errors.New("access token header is malformed")
	}
	var header struct {
		Type      string `json:"typ"`
		Algorithm string `json:"alg"`
	}
	if json.Unmarshal(headerBytes, &header) != nil || !strings.EqualFold(header.Type, expectedType) {
		return errors.New("access token type is invalid")
	}
	for _, algorithm := range algorithms {
		if header.Algorithm == algorithm {
			return nil
		}
	}
	return errors.New("access token signing algorithm is invalid")
}

func audienceContains(raw json.RawMessage, expected string) bool {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single == expected
	}
	var multiple []string
	if json.Unmarshal(raw, &multiple) != nil {
		return false
	}
	for _, audience := range multiple {
		if audience == expected {
			return true
		}
	}
	return false
}

func verifyProof(compact, method, target string, now time.Time) (*proofClaims, string, error) {
	if compact == "" {
		return nil, "", oauthError("invalid_dpop_proof", "DPoP proof is required", 401)
	}
	signed, err := jose.ParseSignedCompact(compact, []jose.SignatureAlgorithm{jose.ES256})
	if err != nil || len(signed.Signatures) != 1 {
		return nil, "", oauthError("invalid_dpop_proof", "DPoP proof is malformed", 401)
	}
	header := signed.Signatures[0].Header
	typeValue, ok := header.ExtraHeaders[jose.HeaderKey("typ")].(string)
	if !ok || !strings.EqualFold(typeValue, "dpop+jwt") || header.Algorithm != string(jose.ES256) || header.JSONWebKey == nil || !header.JSONWebKey.IsPublic() {
		return nil, "", oauthError("invalid_dpop_proof", "DPoP proof header is invalid", 401)
	}
	payload, err := signed.Verify(header.JSONWebKey.Key)
	if err != nil {
		return nil, "", oauthError("invalid_dpop_proof", "DPoP proof signature is invalid", 401)
	}
	var claims proofClaims
	if json.Unmarshal(payload, &claims) != nil || claims.TokenID == "" || claims.IssuedAt == 0 {
		return nil, "", oauthError("invalid_dpop_proof", "DPoP proof claims are invalid", 401)
	}
	if claims.HTTPURI != target || claims.HTTPMethod != strings.ToUpper(method) {
		return nil, "", oauthError("invalid_dpop_proof", "DPoP proof target is invalid", 401)
	}
	issuedAt := time.Unix(claims.IssuedAt, 0)
	if issuedAt.Before(now.Add(-proofLifetime)) || issuedAt.After(now.Add(time.Minute)) {
		return nil, "", oauthError("invalid_dpop_proof", "DPoP proof is stale", 401)
	}
	thumbprint, err := header.JSONWebKey.Thumbprint(crypto.SHA256)
	if err != nil {
		return nil, "", oauthError("invalid_dpop_proof", "DPoP key is invalid", 401)
	}
	return &claims, base64.RawURLEncoding.EncodeToString(thumbprint), nil
}

func (s *databaseReplayStore) Consume(ctx context.Context, thumbprint, jti string, expiresAt time.Time) error {
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).Where("expires_at <= ?", now).Delete(&model.DPoPProof{}).Error; err != nil {
		return err
	}
	proof := model.DPoPProof{KeyThumbprint: thumbprint, JTI: jti, ExpiresAt: expiresAt.UTC(), CreatedAt: now}
	if err := s.db.WithContext(ctx).Create(&proof).Error; err == nil {
		return nil
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.DPoPProof{}).
		Where("key_thumbprint = ? AND jti = ?", thumbprint, jti).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return errProofReplay
	}
	return errors.New("store DPoP proof")
}

type protocolError struct {
	code    string
	message string
	status  int
}

func (e *protocolError) Error() string { return e.message }

func oauthError(code, message string, status int) error {
	return &protocolError{code: code, message: message, status: status}
}
