package auth

import (
	"encoding/base64"
	"testing"

	. "github.com/onsi/gomega"
)

func TestPeekIssuer(t *testing.T) {
	RegisterTestingT(t)

	validHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	fakeSig := base64.RawURLEncoding.EncodeToString([]byte("fake-signature"))

	t.Run("extracts issuer from valid JWT", func(t *testing.T) {
		RegisterTestingT(t)
		payload := base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"https://example.com","sub":"user1"}`))
		token := validHeader + "." + payload + "." + fakeSig
		iss, err := peekIssuer(token)
		Expect(err).NotTo(HaveOccurred())
		Expect(iss).To(Equal("https://example.com"))
	})

	t.Run("returns error for token without three segments", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := peekIssuer("not-a-jwt")
		Expect(err).To(HaveOccurred())
	})

	t.Run("returns error for invalid base64 payload", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := peekIssuer(validHeader + ".!!!invalid!!!." + fakeSig)
		Expect(err).To(HaveOccurred())
	})

	t.Run("returns error for payload without iss claim", func(t *testing.T) {
		RegisterTestingT(t)
		payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user1"}`))
		token := validHeader + "." + payload + "." + fakeSig
		_, err := peekIssuer(token)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no iss claim"))
	})

	t.Run("returns error for empty iss claim", func(t *testing.T) {
		RegisterTestingT(t)
		payload := base64.RawURLEncoding.EncodeToString([]byte(`{"iss":""}`))
		token := validHeader + "." + payload + "." + fakeSig
		_, err := peekIssuer(token)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no iss claim"))
	})
}
