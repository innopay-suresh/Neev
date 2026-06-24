package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClientCertificateBundle contains a device certificate, private key, and fingerprint.
type ClientCertificateBundle struct {
	CertPEM     string `json:"cert_pem"`
	KeyPEM      string `json:"key_pem"`
	Fingerprint string `json:"fingerprint"`
}

// ClientCA manages the local issuing CA used for agent client certificates.
type ClientCA struct {
	certPath string
	keyPath  string
	certPEM  []byte
	key      *ecdsa.PrivateKey
	cert     *x509.Certificate
}

// LoadOrCreateClientCA loads a client issuing CA from disk or creates one if needed.
// If either path is empty, managed issuance is disabled and nil is returned.
func LoadOrCreateClientCA(certPath, keyPath string) (*ClientCA, error) {
	certPath = strings.TrimSpace(certPath)
	keyPath = strings.TrimSpace(keyPath)
	if certPath == "" || keyPath == "" {
		return nil, nil
	}

	if _, err := os.Stat(certPath); err == nil {
		certPEM, err := os.ReadFile(certPath)
		if err != nil {
			return nil, err
		}
		keyPEM, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, err
		}
		block, _ := pem.Decode(certPEM)
		if block == nil {
			return nil, fmt.Errorf("invalid client CA certificate: %s", certPath)
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		keyBlock, _ := pem.Decode(keyPEM)
		if keyBlock == nil {
			return nil, fmt.Errorf("invalid client CA key: %s", keyPath)
		}
		key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
		if err != nil {
			return nil, err
		}
		return &ClientCA{
			certPath: certPath,
			keyPath:  keyPath,
			certPEM:  certPEM,
			key:      key,
			cert:     cert,
		}, nil
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "RemoteAgent Client CA", Organization: []string{"RemoteAgent"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := writeFile(certPath, certPEM, 0o644); err != nil {
		return nil, err
	}
	if err := writeFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, err
	}
	return &ClientCA{
		certPath: certPath,
		keyPath:  keyPath,
		certPEM:  certPEM,
		key:      key,
		cert:     cert,
	}, nil
}

// IssueCertificate creates a per-device client certificate for mTLS enrollment.
func (c *ClientCA) IssueCertificate(commonName, orgID, deviceGroup string) (*ClientCertificateBundle, error) {
	if c == nil || c.cert == nil || c.key == nil {
		return nil, fmt.Errorf("client CA not configured")
	}
	deviceKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	subject := pkix.Name{CommonName: commonName}
	if orgID != "" {
		subject.Organization = []string{orgID}
	}
	if deviceGroup != "" {
		subject.OrganizationalUnit = []string{deviceGroup}
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               subject,
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(2, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, c.cert, &deviceKey.PublicKey, c.key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(deviceKey)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	sum := sha256.Sum256(cert.Raw)
	return &ClientCertificateBundle{
		CertPEM:     string(certPEM),
		KeyPEM:      string(keyPEM),
		Fingerprint: fmt.Sprintf("%x", sum[:]),
	}, nil
}

func (c *ClientCA) CAPEM() string {
	if c == nil {
		return ""
	}
	return string(c.certPEM)
}

// Fingerprint returns the SHA-256 fingerprint of the CA certificate.
func (c *ClientCA) Fingerprint() string {
	if c == nil || c.cert == nil {
		return ""
	}
	sum := sha256.Sum256(c.cert.Raw)
	return fmt.Sprintf("%x", sum[:])
}

func writeFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}
