package e2e_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path"
	"time"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
)

type tlsConfig struct {
	serverTLS *chserver.TLSConfig
	clientTLS *chclient.TLSConfig
	tmpDir    string
}

func (t *tlsConfig) Close() {
	if t.tmpDir != "" {
		os.RemoveAll(t.tmpDir)
	}
}

func newTestTLSConfig() (*tlsConfig, error) {
	tlsConfig := &tlsConfig{}
	_, serverCertPEM, serverKeyPEM, err := certGetCertificate(&certConfig{
		hosts: []string{
			"0.0.0.0",
			"localhost",
		},
		extKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	if err != nil {
		return nil, err
	}
	_, clientCertPEM, clientKeyPEM, err := certGetCertificate(&certConfig{
		extKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		return nil, err
	}

	tlsConfig.tmpDir, err = os.MkdirTemp("", "")
	if err != nil {
		return nil, err
	}

	dirServerCA := path.Join(tlsConfig.tmpDir, "server-ca")
	if err := os.Mkdir(dirServerCA, 0777); err != nil {
		return nil, err
	}
	pathServerCACrt := path.Join(dirServerCA, "client.crt")
	if err := os.WriteFile(pathServerCACrt, clientCertPEM, 0666); err != nil {
		return nil, err
	}

	dirClientCA := path.Join(tlsConfig.tmpDir, "client-ca")
	if err := os.Mkdir(dirClientCA, 0777); err != nil {
		return nil, err
	}
	pathClientCACrt := path.Join(dirClientCA, "server.crt")
	if err := os.WriteFile(pathClientCACrt, serverCertPEM, 0666); err != nil {
		return nil, err
	}

	dirServerCrt := path.Join(tlsConfig.tmpDir, "server-crt")
	if err := os.Mkdir(dirServerCrt, 0777); err != nil {
		return nil, err
	}
	pathServerCrtCrt := path.Join(dirServerCrt, "server.crt")
	if err := os.WriteFile(pathServerCrtCrt, serverCertPEM, 0666); err != nil {
		return nil, err
	}
	pathServerCrtKey := path.Join(dirServerCrt, "server.key")
	if err := os.WriteFile(pathServerCrtKey, serverKeyPEM, 0666); err != nil {
		return nil, err
	}

	dirClientCrt := path.Join(tlsConfig.tmpDir, "client-crt")
	if err := os.Mkdir(dirClientCrt, 0777); err != nil {
		return nil, err
	}
	pathClientCrtCrt := path.Join(dirClientCrt, "client.crt")
	if err := os.WriteFile(pathClientCrtCrt, clientCertPEM, 0666); err != nil {
		return nil, err
	}
	pathClientCrtKey := path.Join(dirClientCrt, "client.key")
	if err := os.WriteFile(pathClientCrtKey, clientKeyPEM, 0666); err != nil {
		return nil, err
	}

	// for self signed cert, it needs the server cert, for real cert, this need to be the trusted CA cert
	tlsConfig.serverTLS = &chserver.TLSConfig{
		CA:   pathServerCACrt,
		Cert: pathServerCrtCrt,
		Key:  pathServerCrtKey,
	}
	tlsConfig.clientTLS = &chclient.TLSConfig{
		CA:   pathClientCACrt,
		Cert: pathClientCrtCrt,
		Key:  pathClientCrtKey,
	}
	return tlsConfig, nil
}

type certConfig struct {
	signCA      *x509.Certificate
	isCA        bool
	hosts       []string
	validFrom   *time.Time
	validFor    *time.Time
	extKeyUsage []x509.ExtKeyUsage
	rsaBits     int
	ecdsaCurve  string
	ed25519Key  bool
}

func certGetCertificate(c *certConfig) (*x509.Certificate, []byte, []byte, error) {
	var err error
	var priv interface{}
	switch c.ecdsaCurve {
	case "":
		if c.ed25519Key {
			_, priv, err = ed25519.GenerateKey(rand.Reader)
		} else {
			rsaBits := c.rsaBits
			if rsaBits == 0 {
				rsaBits = 2048
			}
			priv, err = rsa.GenerateKey(rand.Reader, rsaBits)
		}
	case "P224":
		priv, err = ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	case "P256":
		priv, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "P384":
		priv, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case "P521":
		priv, err = ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	default:
		return nil, nil, nil, fmt.Errorf("Unrecognized elliptic curve: %q", c.ecdsaCurve)
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Failed to generate private key: %v", err)
	}

	// ECDSA, ED25519 and RSA subject keys should have the DigitalSignature
	// KeyUsage bits set in the x509.Certificate template
	keyUsage := x509.KeyUsageDigitalSignature
	// Only RSA subject keys should have the KeyEncipherment KeyUsage bits set. In
	// the context of TLS this KeyUsage is particular to RSA key exchange and
	// authentication.
	if _, isRSA := priv.(*rsa.PrivateKey); isRSA {
		keyUsage |= x509.KeyUsageKeyEncipherment
	}

	notBefore := time.Now()
	if c.validFrom != nil {
		notBefore = *c.validFrom
	}

	notAfter := time.Now().Add(24 * time.Hour)
	if c.validFor != nil {
		notAfter = *c.validFor
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Failed to generate serial number: %v", err)
	}

	cert := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			OrganizationalUnit: []string{"test"},
			Organization:       []string{"Chisel"},
			Country:            []string{"us"},
			Province:           []string{"ma"},
			Locality:           []string{"Boston"},
			CommonName:         "localhost",
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              keyUsage,
		ExtKeyUsage:           c.extKeyUsage,
		BasicConstraintsValid: true,
	}

	for _, h := range c.hosts {
		if ip := net.ParseIP(h); ip != nil {
			cert.IPAddresses = append(cert.IPAddresses, ip)
		} else {
			cert.DNSNames = append(cert.DNSNames, h)
		}
	}

	if c.isCA {
		cert.IsCA = true
		cert.KeyUsage |= x509.KeyUsageCertSign
	}

	ca := cert
	if c.signCA != nil {
		ca = c.signCA
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, ca, certGetPublicKey(priv), priv)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Failed to create certificate: %v", err)
	}

	certPEM := new(bytes.Buffer)
	pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Unable to marshal private key: %v", err)
	}
	certPrivKeyPEM := new(bytes.Buffer)
	pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	})

	return cert, certPEM.Bytes(), certPrivKeyPEM.Bytes(), nil
}

func certGetPublicKey(priv interface{}) interface{} {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	case ed25519.PrivateKey:
		return k.Public().(ed25519.PublicKey)
	default:
		return nil
	}
}
