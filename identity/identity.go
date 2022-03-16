package identity

import (
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"time"

	"github.com/golang/protobuf/proto"
	mspPb "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/msp"
	_ "github.com/hyperledger/fabric/msp"
	"github.com/pkg/errors"

	"github.com/s7techlab/hlf-sdk-go/api"
)

type (
	identity struct {
		privateKey  interface{}
		publicKey   interface{}
		certificate *x509.Certificate
		mspId       string
	}
	signingIdentity struct {
		identity    *identity
		cryptoSuite api.CryptoSuite
	}
)

func (i *identity) GetSigningIdentity(cs api.CryptoSuite) msp.SigningIdentity {
	return &signingIdentity{
		identity:    i,
		cryptoSuite: cs,
	}
}

func (i *identity) GetMSPIdentifier() string {
	return i.mspId
}

func (i *identity) GetIdentifier() *msp.IdentityIdentifier {
	return &msp.IdentityIdentifier{
		Mspid: i.mspId,
		Id:    i.certificate.Subject.CommonName,
	}
}

func (s *signingIdentity) Anonymous() bool {
	return false
}

// ExpiresAt returns date of certificate expiration
func (s *signingIdentity) ExpiresAt() time.Time {
	return s.identity.certificate.NotAfter
}

func (s *signingIdentity) GetIdentifier() *msp.IdentityIdentifier {
	return s.identity.GetIdentifier()
}

// GetMSPIdentifier returns current MspID of identity
func (s *signingIdentity) GetMSPIdentifier() string {
	return s.identity.GetMSPIdentifier()
}

func (s *signingIdentity) Validate() error {
	// TODO
	return nil
}

func (s *signingIdentity) GetOrganizationalUnits() []*msp.OUIdentifier {
	// TODO
	return nil
}

func (s *signingIdentity) Verify(msg []byte, sig []byte) error {
	return s.cryptoSuite.Verify(s.identity.publicKey, msg, sig)
}

func (s *signingIdentity) Serialize() ([]byte, error) {
	pb := &pem.Block{Bytes: s.identity.certificate.Raw, Type: "CERTIFICATE"}
	pemBytes := pem.EncodeToMemory(pb)
	if pemBytes == nil {
		return nil, errors.New("encoding of identity failed")
	}

	sId := &mspPb.SerializedIdentity{Mspid: s.identity.mspId, IdBytes: pemBytes}
	idBytes, err := proto.Marshal(sId)
	if err != nil {
		return nil, err
	}

	return idBytes, nil
}

func (s *signingIdentity) SatisfiesPrincipal(principal *mspPb.MSPPrincipal) error {
	panic("implement me")
}

func (s *signingIdentity) Sign(msg []byte) ([]byte, error) {
	return s.cryptoSuite.Sign(msg, s.identity.privateKey)
}

func (s *signingIdentity) GetPublicVersion() msp.Identity {
	return nil
}

func FromCertKeyPath(mspId string, certPath string, keyPath string) (api.Identity, error) {
	certPEMBytes, err := ioutil.ReadFile(certPath)
	if err != nil {
		return nil, errors.Wrap(err, `failed to open certificate`)
	}

	keyPEMBytes, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, errors.Wrap(err, `failed to open private key`)
	}

	return FromBytes(mspId, certPEMBytes, keyPEMBytes)
}

func FromBytes(mspId string, certBytes []byte, keyBytes []byte) (api.Identity, error) {
	certPEM, _ := pem.Decode(certBytes)
	if certPEM == nil {
		return nil, errors.Wrap(api.ErrInvalidPEMStructure, `failed to decode certificate`)
	}

	keyPEM, _ := pem.Decode(keyBytes)
	if keyPEM == nil {
		return nil, errors.Wrap(api.ErrInvalidPEMStructure, `failed to decode private key`)
	}

	cert, err := x509.ParseCertificate(certPEM.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, `failed to parse x509 certificate`)
	}

	key, err := x509.ParsePKCS8PrivateKey(keyPEM.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, `failed to parse private key`)
	}

	return New(mspId, cert, key)
}

func New(mspId string, cert *x509.Certificate, privateKey interface{}) (api.Identity, error) {
	return &identity{
		mspId:       mspId,
		privateKey:  privateKey,
		certificate: cert,
		publicKey:   cert.PublicKey}, nil
}
func FromMSPPath(mspId string, mspPath string) (api.Identity, error) {
	cert, key, err := LoadKeyPairFromMSP(mspPath)
	if err != nil {
		return nil, err
	}

	return New(mspId, cert, key)
}