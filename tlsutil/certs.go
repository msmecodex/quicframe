// Package tlsutil provides TLS configuration helpers for QuicFrame.
package tlsutil

import (
	"crypto/sha256"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"encoding/hex"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

const webTransportDevCertValidity = 13 * 24 * time.Hour

// SelfSigned generates a self-signed TLS certificate valid for the given
// hostnames and IPs.  Suitable for development and testing.
func SelfSigned(hosts ...string) (*tls.Config, error) {
	certPEM, keyPEM, err := generateSelfSignedPEM(hosts...)
	if err != nil {
		return nil, err
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("tlsutil: X509KeyPair: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13, // QUIC requires TLS 1.3
	}, nil
}

// AutocertConfig holds parameters for Let's Encrypt certificate provisioning.
type AutocertConfig struct {
	// Domains is the list of domain names to obtain certificates for.
	Domains []string

	// CacheDir is the directory for certificate caching.
	// Defaults to "/var/cache/quicframe/autocert".
	CacheDir string

	// Email is the contact address for ACME account registration.
	Email string

	// Staging uses the Let's Encrypt staging environment (for testing).
	Staging bool
}

// Autocert returns a *tls.Config that automatically provisions and renews
// TLS certificates from Let's Encrypt via the ACME protocol.
//
// The server must be publicly reachable on port 443 for the HTTP-01 or
// TLS-ALPN-01 challenge to succeed.
func Autocert(cfg AutocertConfig) (*tls.Config, *autocert.Manager, error) {
	if len(cfg.Domains) == 0 {
		return nil, nil, fmt.Errorf("tlsutil: at least one domain is required")
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = "/var/cache/quicframe/autocert"
	}

	m := &autocert.Manager{
		Cache:      autocert.DirCache(cfg.CacheDir),
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(cfg.Domains...),
		Email:      cfg.Email,
	}

	if cfg.Staging {
		m.Client = &acme.Client{
			DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
		}
	}

	tlsCfg := m.TLSConfig()
	tlsCfg.MinVersion = tls.VersionTLS13

	return tlsCfg, m, nil
}

// FromFiles loads a TLS certificate and private key from PEM files.
func FromFiles(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("tlsutil: load key pair: %w", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// LoadOrCreateSelfSigned loads an existing key pair from disk or creates one.
// The generated certificate is persisted so local browser trust / certificate
// pinning can remain stable across restarts.
func LoadOrCreateSelfSigned(certFile, keyFile string, hosts ...string) (*tls.Config, error) {
	if _, err := os.Stat(certFile); err == nil {
		if _, err := os.Stat(keyFile); err == nil {
			cfg, err := FromFiles(certFile, keyFile)
			if err == nil {
				ok, checkErr := webTransportCompatibleLeaf(cfg)
				if checkErr == nil && ok {
					return cfg, nil
				}
			}
		}
	}

	certPEM, keyPEM, err := generateSelfSignedPEM(hosts...)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(certFile), 0o755); err != nil {
		return nil, fmt.Errorf("tlsutil: create cert dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyFile), 0o755); err != nil {
		return nil, fmt.Errorf("tlsutil: create key dir: %w", err)
	}
	if err := os.WriteFile(certFile, certPEM, 0o644); err != nil {
		return nil, fmt.Errorf("tlsutil: write cert: %w", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		return nil, fmt.Errorf("tlsutil: write key: %w", err)
	}

	return FromFiles(certFile, keyFile)
}

func generateSelfSignedPEM(hosts ...string) ([]byte, []byte, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("tlsutil: generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("tlsutil: generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{Organization: []string{"QuicFrame Dev"}},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(webTransportDevCertValidity),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("tlsutil: create certificate: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("tlsutil: marshal key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

func webTransportCompatibleLeaf(cfg *tls.Config) (bool, error) {
	if cfg == nil || len(cfg.Certificates) == 0 {
		return false, fmt.Errorf("tlsutil: no certificates configured")
	}
	chain := cfg.Certificates[0].Certificate
	if len(chain) == 0 {
		return false, fmt.Errorf("tlsutil: certificate chain is empty")
	}

	leaf, err := x509.ParseCertificate(chain[0])
	if err != nil {
		return false, fmt.Errorf("tlsutil: parse leaf certificate: %w", err)
	}

	validity := leaf.NotAfter.Sub(leaf.NotBefore)
	if validity > webTransportDevCertValidity {
		return false, nil
	}

	now := time.Now()
	if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return false, nil
	}

	return true, nil
}

// CertificateSHA256 returns the SHA-256 digest of the leaf certificate.
func CertificateSHA256(cfg *tls.Config) ([]byte, error) {
	if cfg == nil || len(cfg.Certificates) == 0 {
		return nil, fmt.Errorf("tlsutil: no certificates configured")
	}

	chain := cfg.Certificates[0].Certificate
	if len(chain) == 0 {
		return nil, fmt.Errorf("tlsutil: certificate chain is empty")
	}

	sum := sha256.Sum256(chain[0])
	return sum[:], nil
}

// CertificateSHA256Hex returns the SHA-256 digest of the leaf certificate as hex.
func CertificateSHA256Hex(cfg *tls.Config) (string, error) {
	sum, err := CertificateSHA256(cfg)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sum), nil
}
