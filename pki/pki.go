package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// Identity represents a unique node identity with its certificate and private key.
type Identity struct {
	NodeID     string
	CertFile   string
	KeyFile    string
	Cert       *x509.Certificate
	PrivateKey *ecdsa.PrivateKey
}

// Manager handles the lifecycle of node identities.
type Manager struct {
	baseDir string
}

// NewManager creates a new PKI manager with a base directory for storage.
func NewManager(baseDir string) *Manager {
	return &Manager{baseDir: baseDir}
}

// LoadOrCreateIdentity loads an existing identity or generates a new one.
func (m *Manager) LoadOrCreateIdentity(nodeID string, hosts ...string) (*Identity, error) {
	nodeDir := filepath.Join(m.baseDir, nodeID)
	certFile := filepath.Join(nodeDir, "cert.pem")
	keyFile := filepath.Join(nodeDir, "key.pem")

	if _, err := os.Stat(certFile); err == nil {
		if _, err := os.Stat(keyFile); err == nil {
			id, err := m.LoadIdentity(nodeID)
			if err == nil {
				// Check if it needs rotation (e.g. for WebTransport's 14-day rule)
				if time.Until(id.Cert.NotAfter) > 24*time.Hour {
					return id, nil
				}
			}
		}
	}

	return m.GenerateIdentity(nodeID, hosts...)
}

// LoadIdentity loads an identity from disk.
func (m *Manager) LoadIdentity(nodeID string) (*Identity, error) {
	nodeDir := filepath.Join(m.baseDir, nodeID)
	certFile := filepath.Join(nodeDir, "cert.pem")
	keyFile := filepath.Join(nodeDir, "key.pem")

	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("pki: failed to decode certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("pki: failed to decode key PEM")
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}

	return &Identity{
		NodeID:     nodeID,
		CertFile:   certFile,
		KeyFile:    keyFile,
		Cert:       cert,
		PrivateKey: key,
	}, nil
}

// GenerateIdentity creates a new self-signed identity.
func (m *Manager) GenerateIdentity(nodeID string, hosts ...string) (*Identity, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("pki: generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("pki: generate serial: %w", err)
	}

	// WebTransport requires max 14 days validity for self-signed certs.
	// We use 13 days to be safe.
	notBefore := time.Now().Add(-time.Minute)
	notAfter := notBefore.Add(13 * 24 * time.Hour)

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"QuicFrame Managed PKI"},
			CommonName:   nodeID,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	for _, h := range hosts {
		template.DNSNames = append(template.DNSNames, h)
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, fmt.Errorf("pki: create certificate: %w", err)
	}

	nodeDir := filepath.Join(m.baseDir, nodeID)
	if err := os.MkdirAll(nodeDir, 0755); err != nil {
		return nil, fmt.Errorf("pki: create node dir: %w", err)
	}

	certFile := filepath.Join(nodeDir, "cert.pem")
	keyFile := filepath.Join(nodeDir, "key.pem")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, _ := x509.MarshalECPrivateKey(priv)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(certFile, certPEM, 0644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		return nil, err
	}

	cert, _ := x509.ParseCertificate(certDER)

	return &Identity{
		NodeID:     nodeID,
		CertFile:   certFile,
		KeyFile:    keyFile,
		Cert:       cert,
		PrivateKey: priv,
	}, nil
}

// StartRotationWatcher starts a background goroutine to rotate certificates
// before they expire.
func (m *Manager) StartRotationWatcher(interval time.Duration, nodeID string, hosts ...string) chan struct{} {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				id, err := m.LoadIdentity(nodeID)
				if err == nil {
					// Rotate if less than 24 hours remaining
					if time.Until(id.Cert.NotAfter) < 24*time.Hour {
						_, _ = m.GenerateIdentity(nodeID, hosts...)
					}
				}
			case <-stop:
				return
			}
		}
	}()
	return stop
}
