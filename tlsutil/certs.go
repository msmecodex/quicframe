// Package tlsutil provides TLS configuration helpers for QuicFrame.
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// SelfSigned generates a self-signed TLS certificate valid for the given
// hostnames and IPs.  Suitable for development and testing.
func SelfSigned(hosts ...string) (*tls.Config, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("tlsutil: generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("tlsutil: generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{Organization: []string{"QuicFrame Dev"}},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
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
		return nil, fmt.Errorf("tlsutil: create certificate: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("tlsutil: marshal key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

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
