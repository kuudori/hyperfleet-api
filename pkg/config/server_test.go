package config

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestJWTConfig_Validate(t *testing.T) {
	RegisterTestingT(t)

	t.Run("disabled JWT requires nothing", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{Enabled: false}
		Expect(cfg.Validate()).To(Succeed())
	})

	t.Run("enabled JWT with legacy issuer URL passes", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{
			Enabled:       true,
			IssuerURL:     "https://sso.example.com/auth/realms/test",
			IdentityClaim: "email",
		}
		Expect(cfg.Validate()).To(Succeed())
	})

	t.Run("enabled JWT without identity claim fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{Enabled: true, IssuerURL: "https://sso.example.com/auth/realms/test", IdentityClaim: ""}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("identity_claim"))
	})

	t.Run("enabled JWT without any issuer fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{Enabled: true, IdentityClaim: "email"}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("at least one issuer"))
	})

	t.Run("enabled JWT with issuers list passes", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{
			Enabled:       true,
			IdentityClaim: "email",
			Issuers: []IssuerConfig{
				{
					IssuerURL:  "https://sso.example.com",
					Type:       "jwks",
					JWKCertURL: "https://sso.example.com/.well-known/jwks.json",
				},
			},
		}
		Expect(cfg.Validate()).To(Succeed())
	})

	t.Run("jwks issuer without cert config fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{
			Enabled:       true,
			IdentityClaim: "email",
			Issuers: []IssuerConfig{
				{IssuerURL: "https://sso.example.com", Type: "jwks"},
			},
		}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("jwk_cert_file or jwk_cert_url"))
	})

	t.Run("k8s-token-review issuer passes without cert config", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{
			Enabled:       true,
			IdentityClaim: "email",
			Issuers: []IssuerConfig{
				{IssuerURL: "https://kubernetes.default.svc", Type: "k8s-token-review", Audience: "hyperfleet-api"},
			},
		}
		Expect(cfg.Validate()).To(Succeed())
	})
}

func TestJWTConfig_ResolvedIssuers(t *testing.T) {
	RegisterTestingT(t)

	t.Run("returns issuers list when set", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{
			IdentityClaim: "email",
			Issuers: []IssuerConfig{
				{IssuerURL: "https://a.com", Type: "jwks", JWKCertURL: "https://a.com/jwks"},
				{IssuerURL: "https://b.com", Type: "k8s-token-review", Audience: "hyperfleet-api", IdentityClaim: "sub"},
			},
		}
		resolved := cfg.ResolvedIssuers(JWKConfig{})
		Expect(resolved).To(HaveLen(2))
		Expect(resolved[0].IdentityClaim).To(Equal("email")) // inherits global default
		Expect(resolved[1].IdentityClaim).To(Equal("sub"))   // keeps its own
	})

	t.Run("synthesizes from legacy fields", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{
			IssuerURL:     "https://sso.example.com",
			Audience:      "my-api",
			IdentityClaim: "email",
		}
		jwk := JWKConfig{CertURL: "https://sso.example.com/.well-known/jwks.json"}
		resolved := cfg.ResolvedIssuers(jwk)
		Expect(resolved).To(HaveLen(1))
		Expect(resolved[0].IssuerURL).To(Equal("https://sso.example.com"))
		Expect(resolved[0].Type).To(Equal("jwks"))
		Expect(resolved[0].Audience).To(Equal("my-api"))
		Expect(resolved[0].JWKCertURL).To(Equal("https://sso.example.com/.well-known/jwks.json"))
	})

	t.Run("returns nil when no issuers and no legacy", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{IdentityClaim: "email"}
		resolved := cfg.ResolvedIssuers(JWKConfig{})
		Expect(resolved).To(BeNil())
	})
}

func TestServerConfig_ValidateIdentityHeader(t *testing.T) {
	RegisterTestingT(t)

	t.Run("empty identity header requires nothing", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &ServerConfig{}
		Expect(cfg.ValidateIdentityHeader()).To(Succeed())
	})

	t.Run("forbidden header name fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &ServerConfig{IdentityHeader: "Authorization"}
		err := cfg.ValidateIdentityHeader()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not allowed"))
	})

	t.Run("valid header name passes", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &ServerConfig{IdentityHeader: "X-HyperFleet-Identity"}
		Expect(cfg.ValidateIdentityHeader()).To(Succeed())
	})
}

func TestTimeoutsConfig_Validate(t *testing.T) {
	RegisterTestingT(t)

	t.Run("valid timeouts pass", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := TimeoutsConfig{Read: 5_000_000_000, Write: 30_000_000_000}
		Expect(cfg.Validate()).To(Succeed())
	})

	t.Run("read timeout too short fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := TimeoutsConfig{Read: 500_000_000, Write: 30_000_000_000}
		Expect(cfg.Validate()).To(HaveOccurred())
	})
}
