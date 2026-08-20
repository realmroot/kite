package resourceapi

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-jose/go-jose/v4"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
)

type issuerFixture struct {
	origin string
	key    *rsa.PrivateKey
	kid    string
}

func TestResourceServerDiscoveryAndDPoPAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	issuer := newIssuerFixture(t)
	db, err := gorm.Open(sqlite.Open("file:resource-api?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Cluster{}, &model.DPoPProof{}, &model.ResourceAccessAudit{}); err != nil {
		t.Fatal(err)
	}
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	if err := db.Create(&model.Cluster{Name: "development", ConnectionMode: "direct", Enable: true, IsDefault: true}).Error; err != nil {
		t.Fatal(err)
	}

	resource := "https://kite.example.test/api/agent/v1"
	server, err := New(context.Background(), resource, issuer.origin, []string{"agent-client"}, []string{"RS256"}, &cluster.ClusterManager{}, db)
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	server.Register(engine)

	metadata := performRequest(engine, "/.well-known/oauth-protected-resource/api/agent/v1", nil)
	if metadata.Code != http.StatusOK || !strings.Contains(metadata.Body.String(), `"dpop_bound_access_tokens_required":true`) {
		t.Fatalf("metadata response = %d %s", metadata.Code, metadata.Body.String())
	}
	description := performRequest(engine, "/api/agent/v1", nil)
	if description.Code != http.StatusOK || !strings.Contains(description.Header().Get("Link"), `rel="service-desc"`) {
		t.Fatalf("service description = %d %q", description.Code, description.Header().Get("Link"))
	}
	contract := performRequest(engine, "/api/agent/openapi.json", nil)
	if contract.Code != http.StatusOK || !strings.Contains(contract.Body.String(), `"operationId":"listClusters"`) {
		t.Fatalf("OpenAPI response = %d %s", contract.Code, contract.Body.String())
	}
	var openAPI map[string]any
	if err := json.Unmarshal(contract.Body.Bytes(), &openAPI); err != nil {
		t.Fatalf("decode OpenAPI contract: %v", err)
	}
	paths := openAPI["paths"].(map[string]any)
	kubernetesPath := paths["/clusters/{cluster}/kubernetes/{path}"].(map[string]any)
	post := kubernetesPath["post"].(map[string]any)
	if _, ok := post["requestBody"]; !ok {
		t.Fatal("Kubernetes write operation must publish its request body")
	}

	target := resource + "/clusters"
	dpopKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	thumbprint := jwkThumbprint(t, &dpopKey.PublicKey)
	token := issuer.signToken(t, resource, thumbprint, ScopeClustersRead)
	proof := signProof(t, dpopKey, http.MethodGet, target, token, "proof-1", time.Now())
	headers := map[string]string{"Authorization": "DPoP " + token, "DPoP": proof}

	response := performRequest(engine, "/api/agent/v1/clusters", headers)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"development"`) {
		t.Fatalf("cluster response = %d %s", response.Code, response.Body.String())
	}
	var auditCount int64
	if err := db.Model(&model.ResourceAccessAudit{}).Where("agent_subject = ? AND controller_subject = ? AND status = ?", "agent-1", "user-1", 200).Count(&auditCount).Error; err != nil || auditCount != 1 {
		t.Fatalf("audit count = %d, err = %v", auditCount, err)
	}

	replay := performRequest(engine, "/api/agent/v1/clusters", headers)
	if replay.Code != http.StatusUnauthorized || !strings.Contains(replay.Body.String(), "already used") {
		t.Fatalf("replay response = %d %s", replay.Code, replay.Body.String())
	}

	wrongScopeToken := issuer.signToken(t, resource, thumbprint, ScopeKubernetesRead)
	wrongScopeProof := signProof(t, dpopKey, http.MethodGet, target, wrongScopeToken, "proof-2", time.Now())
	forbidden := performRequest(engine, "/api/agent/v1/clusters", map[string]string{
		"Authorization": "DPoP " + wrongScopeToken, "DPoP": wrongScopeProof,
	})
	if forbidden.Code != http.StatusForbidden || !strings.Contains(forbidden.Body.String(), "insufficient_scope") {
		t.Fatalf("scope response = %d %s", forbidden.Code, forbidden.Body.String())
	}

	missingProof := performRequest(engine, "/api/agent/v1/clusters", map[string]string{"Authorization": "DPoP " + token})
	if missingProof.Code != http.StatusUnauthorized || missingProof.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("missing proof response = %d %s", missingProof.Code, missingProof.Body.String())
	}
}

func newIssuerFixture(t *testing.T) *issuerFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &issuerFixture{key: key, kid: "test-key"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer": fixture.origin, "authorization_endpoint": fixture.origin + "/authorize",
				"token_endpoint": fixture.origin + "/token", "jwks_uri": fixture.origin + "/jwks",
				"id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/jwks":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: fixture.kid, Algorithm: "RS256", Use: "sig"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	fixture.origin = server.URL
	t.Cleanup(server.Close)
	return fixture
}

func (f *issuerFixture) signToken(t *testing.T, audience, thumbprint, scope string) string {
	t.Helper()
	options := (&jose.SignerOptions{}).WithType("at+jwt").WithHeader("kid", f.kid)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: f.key}, options)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"iss": f.origin, "sub": "user-1", "aud": audience, "exp": time.Now().Add(5 * time.Minute).Unix(),
		"iat": time.Now().Unix(), "jti": "access-" + scope, "client_id": "agent-client", "scope": scope,
		"cnf": map[string]string{"jkt": thumbprint}, "act": map[string]string{"iss": f.origin, "sub": "agent-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	signed, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	compact, err := signed.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return compact
}

func signProof(t *testing.T, key *ecdsa.PrivateKey, method, target, token, jti string, issuedAt time.Time) string {
	t.Helper()
	publicJWK := jose.JSONWebKey{Key: &key.PublicKey, Algorithm: "ES256", Use: "sig"}
	options := (&jose.SignerOptions{}).WithType("dpop+jwt").WithHeader("jwk", publicJWK)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: key}, options)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(token))
	payload, err := json.Marshal(map[string]any{
		"htm": method, "htu": target, "iat": issuedAt.Unix(), "jti": jti,
		"ath": base64.RawURLEncoding.EncodeToString(digest[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	signed, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	compact, err := signed.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return compact
}

func jwkThumbprint(t *testing.T, key *ecdsa.PublicKey) string {
	t.Helper()
	thumbprint, err := (&jose.JSONWebKey{Key: key}).Thumbprint(crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(thumbprint)
}

func performRequest(handler http.Handler, target string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
