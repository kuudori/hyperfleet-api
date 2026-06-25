package config

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validation"
)

// ServerConfig holds HTTP/HTTPS server configuration
// Follows HyperFleet Configuration Standard
type ServerConfig struct {
	JWK               JWKConfig      `mapstructure:"jwk" json:"jwk" validate:"required"`
	Hostname          string         `mapstructure:"hostname" json:"hostname" validate:"omitempty,hostname|ip"`
	Host              string         `mapstructure:"host" json:"host" validate:"required,hostname|ip"`
	OpenAPISchemaPath string         `mapstructure:"openapi_schema_path" json:"openapi_schema_path"`
	IdentityHeader    string         `mapstructure:"identity_header" json:"identity_header"`
	TLS               TLSConfig      `mapstructure:"tls" json:"tls" validate:"required"`
	JWT               JWTConfig      `mapstructure:"jwt" json:"jwt" validate:"required"`
	Timeouts          TimeoutsConfig `mapstructure:"timeouts" json:"timeouts" validate:"required"`
	Port              int            `mapstructure:"port" json:"port" validate:"required,min=1,max=65535"`
}

// TimeoutsConfig holds HTTP timeout configuration
type TimeoutsConfig struct {
	Read  time.Duration `mapstructure:"read" json:"read" validate:"required"`
	Write time.Duration `mapstructure:"write" json:"write" validate:"required"`
}

// Validate validates timeout durations
func (c *TimeoutsConfig) Validate() error {
	if c.Read < 1*time.Second {
		return fmt.Errorf("read timeout must be at least 1 second, got %v", c.Read)
	}
	if c.Write < 1*time.Second {
		return fmt.Errorf("write timeout must be at least 1 second, got %v", c.Write)
	}
	return nil
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	CertFile string `mapstructure:"cert_file" json:"cert_file" validate:"omitempty,filepath"`
	KeyFile  string `mapstructure:"key_file" json:"key_file" validate:"omitempty,filepath"`
	Enabled  bool   `mapstructure:"enabled" json:"enabled"`
}

// Validate validates TLS configuration
func (c *TLSConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	// When TLS is enabled, both cert and key files must be provided
	if c.CertFile == "" {
		return fmt.Errorf("TLS cert file is required when TLS is enabled")
	}
	if c.KeyFile == "" {
		return fmt.Errorf("TLS key file is required when TLS is enabled")
	}
	return nil
}

const (
	IssuerTypeJWKS           = "jwks"
	IssuerTypeK8sTokenReview = "k8s-token-review" //nolint:gosec // not a credential
)

// IssuerConfig describes a single token issuer and how to validate its tokens.
type IssuerConfig struct {
	IssuerURL     string `mapstructure:"issuer_url" json:"issuer_url" validate:"required,url"`
	Type          string `mapstructure:"type" json:"type" validate:"required,oneof=jwks k8s-token-review"`
	Audience      string `mapstructure:"audience" json:"audience"`
	IdentityClaim string `mapstructure:"identity_claim" json:"identity_claim"`
	JWKCertFile   string `mapstructure:"jwk_cert_file" json:"jwk_cert_file"`
	JWKCertURL    string `mapstructure:"jwk_cert_url" json:"jwk_cert_url"`
	Optional      bool   `mapstructure:"optional" json:"optional"`
}

func (ic *IssuerConfig) Validate() error {
	if ic.IssuerURL == "" {
		return fmt.Errorf("issuer_url is required")
	}
	switch ic.Type {
	case IssuerTypeJWKS:
		if ic.JWKCertFile == "" && ic.JWKCertURL == "" {
			return fmt.Errorf(
				"issuer %q (type %s) requires jwk_cert_file or jwk_cert_url",
				ic.IssuerURL, IssuerTypeJWKS,
			)
		}
	case IssuerTypeK8sTokenReview:
		if ic.Audience == "" {
			return fmt.Errorf(
				"issuer %q (type %s) requires audience",
				ic.IssuerURL, IssuerTypeK8sTokenReview,
			)
		}
	default:
		return fmt.Errorf(
			"issuer %q has unknown type %q (must be %s or %s)",
			ic.IssuerURL, ic.Type, IssuerTypeJWKS, IssuerTypeK8sTokenReview,
		)
	}
	return nil
}

// JWTConfig holds JWT authentication configuration.
// Supports multiple issuers via the Issuers list. For backward compatibility,
// legacy single-issuer fields (IssuerURL, Audience) are accepted when Issuers is empty.
type JWTConfig struct {
	IdentityClaim string         `mapstructure:"identity_claim" json:"identity_claim"`
	IssuerURL     string         `mapstructure:"issuer_url" json:"issuer_url" validate:"omitempty,url"`
	Audience      string         `mapstructure:"audience" json:"audience"`
	Issuers       []IssuerConfig `mapstructure:"issuers" json:"issuers"`
	Enabled       bool           `mapstructure:"enabled" json:"enabled"`
}

// ResolvedIssuers returns the effective issuer list. When Issuers is empty but legacy
// IssuerURL is set, a single JWKS issuer is synthesized from the legacy fields.
func (c *JWTConfig) ResolvedIssuers(jwk JWKConfig) []IssuerConfig {
	if len(c.Issuers) > 0 {
		if c.IssuerURL != "" {
			slog.Warn("jwt.issuers and legacy jwt.issuer_url both set; issuer_url ignored")
		}
		resolved := make([]IssuerConfig, len(c.Issuers))
		for i, ic := range c.Issuers {
			resolved[i] = ic
			if resolved[i].IdentityClaim == "" {
				resolved[i].IdentityClaim = c.IdentityClaim
			}
		}
		return resolved
	}

	if c.IssuerURL == "" {
		return nil
	}

	return []IssuerConfig{{
		IssuerURL:     c.IssuerURL,
		Type:          IssuerTypeJWKS,
		Audience:      c.Audience,
		IdentityClaim: c.IdentityClaim,
		JWKCertFile:   jwk.CertFile,
		JWKCertURL:    jwk.CertURL,
	}}
}

func (c *JWTConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.IdentityClaim == "" {
		return fmt.Errorf("server.jwt.identity_claim is required when jwt is enabled")
	}
	if len(c.Issuers) == 0 && c.IssuerURL == "" {
		return fmt.Errorf("server.jwt requires at least one issuer (via issuers list or legacy issuer_url)")
	}
	seen := make(map[string]int, len(c.Issuers))
	for i := range c.Issuers {
		if err := c.Issuers[i].Validate(); err != nil {
			return fmt.Errorf("server.jwt.issuers[%d]: %w", i, err)
		}
		if prev, dup := seen[c.Issuers[i].IssuerURL]; dup {
			return fmt.Errorf(
				"server.jwt.issuers[%d]: duplicate issuer_url %q (first at index %d)",
				i, c.Issuers[i].IssuerURL, prev,
			)
		}
		seen[c.Issuers[i].IssuerURL] = i
	}
	return nil
}

// ValidateIdentityHeader validates the identity header name if set.
func (s *ServerConfig) ValidateIdentityHeader() error {
	if s.IdentityHeader == "" {
		return nil
	}
	if validation.IsForbiddenIdentityHeaderName(s.IdentityHeader) {
		return fmt.Errorf("server.identity_header %q is not allowed", s.IdentityHeader)
	}
	return nil
}

// JWKConfig holds JWK certificate configuration
type JWKConfig struct {
	CertFile string `mapstructure:"cert_file" json:"cert_file" validate:"omitempty,filepath"`
	CertURL  string `mapstructure:"cert_url" json:"cert_url" validate:"omitempty,url"`
}

// NewServerConfig returns default ServerConfig values
// These defaults can be overridden by config file, env vars, or CLI flags
func NewServerConfig() *ServerConfig {
	return &ServerConfig{
		Hostname:          "",
		Host:              "localhost",
		Port:              8000,
		OpenAPISchemaPath: "openapi/openapi.yaml",
		Timeouts: TimeoutsConfig{
			Read:  5 * time.Second,
			Write: 30 * time.Second,
		},
		TLS: TLSConfig{
			Enabled:  false,
			CertFile: "",
			KeyFile:  "",
		},
		JWT: JWTConfig{
			Enabled:       true,
			IssuerURL:     "",
			Audience:      "",
			IdentityClaim: "email",
		},
		JWK: JWKConfig{
			CertFile: "",
			CertURL:  "",
		},
	}
}

// ============================================================
// Convenience Accessor Methods
// ============================================================

// BindAddress returns bind address in host:port format
// Uses net.JoinHostPort to correctly handle IPv6 addresses
func (s *ServerConfig) BindAddress() string {
	return net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
}
