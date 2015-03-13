package chisel

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"log"
	"math/big"
	"time"
)

func GenerateKeys() ([]byte, []byte) {

	log.Println("generating tls certs")

	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1337),
		Subject: pkix.Name{
			Country:            []string{"Neuland"},
			Organization:       []string{"qwertz"},
			OrganizationalUnit: []string{"qwertz"},
		},
		Issuer: pkix.Name{
			Country:            []string{"Neuland"},
			Organization:       []string{"Skynet"},
			OrganizationalUnit: []string{"Computer Emergency Response Team"},
			Locality:           []string{"Neuland"},
			Province:           []string{"Neuland"},
			StreetAddress:      []string{"Mainstreet 23"},
			PostalCode:         []string{"12345"},
			SerialNumber:       "23",
			CommonName:         "23",
		},
		SignatureAlgorithm:    x509.SHA512WithRSA,
		PublicKeyAlgorithm:    x509.ECDSA,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, 10),
		SubjectKeyId:          []byte{1, 2, 3, 4, 5},
		BasicConstraintsValid: true,
		IsCA:        true,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}

	priv, _ := rsa.GenerateKey(rand.Reader, 1024)
	pub := &priv.PublicKey
	caB, _ := x509.CreateCertificate(rand.Reader, ca, ca, pub, priv)

	keyB := x509.MarshalPKCS1PrivateKey(priv)

	return caB, keyB
}

func TLSConfig() *tls.Config {

	caB, keyB := GenerateKeys()

	ca, _ := x509.ParseCertificate(caB)
	key, _ := x509.ParsePKCS1PrivateKey(keyB)
	pool := x509.NewCertPool()
	pool.AddCert(ca)

	config := &tls.Config{
		Certificates: []tls.Certificate{
			tls.Certificate{
				Certificate: [][]byte{caB},
				PrivateKey:  key,
			},
		},
		MinVersion: tls.VersionTLS11,
	}
	config.Rand = rand.Reader

	return config
}
