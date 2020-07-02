package calypso

import (
	"bytes"

	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"

	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/ledger/arc"
	"go.dedis.ch/dela/ledger/arc/darc"
	"go.dedis.ch/dela/lottery"
	"go.dedis.ch/dela/lottery/calypso/json"
	"go.dedis.ch/dela/lottery/storage"
	"go.dedis.ch/dela/lottery/storage/inmemory"
	serdej "go.dedis.ch/dela/serde/json"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
)

const (
	keySize = 32

	// ArcRuleUpdate defines the rule to update the arc. This rule must be set
	// at the write creation to allow the arc to be latter updated.
	ArcRuleUpdate = "calypso_update"
	// ArcRuleRead defines the arc rule to read a value
	ArcRuleRead = "calypso_read"
)

// Suite is the Kyber suite for Pedersen.
var suite = suites.MustFind("Ed25519")

// NewCalypso creates a new Calypso
func NewCalypso(dkg dkg.DKG) *Calypso {
	return &Calypso{
		dkg:     dkg,
		storage: inmemory.NewInMemory(),
	}
}

// Calypso is a wrapper around DKG to provide a storage and authorization layer
//
// implements lottery.Secret
type Calypso struct {
	dkg      dkg.DKG
	dkgActor dkg.Actor
	storage  storage.KeyValue
}

// Listen implements lottery.Secret
func (c *Calypso) Listen() error {
	if c.dkgActor != nil {
		return xerrors.Errorf("listen has already been called")
	}

	actor, err := c.dkg.Listen()
	if err != nil {
		return xerrors.Errorf("failed to listen dkg: %v", err)
	}

	c.dkgActor = actor

	return nil
}

// Setup implements lottery.Secret
func (c *Calypso) Setup(ca crypto.CollectiveAuthority,
	threshold int) (pubKey kyber.Point, err error) {

	if c.dkgActor == nil {
		return nil, xerrors.Errorf("dkg actor is nil, did you call Listen() first?")
	}

	pubKey, err = c.dkgActor.Setup(ca, threshold)
	if err != nil {
		return nil, xerrors.Errorf("failed to setup dkg: %v", err)
	}

	return pubKey, nil
}

// GetPublicKey implements lottery.Secret
func (c *Calypso) GetPublicKey() (kyber.Point, error) {
	if c.dkgActor == nil {
		return nil, xerrors.Errorf("listen has not already been called")
	}

	pubKey, err := c.dkgActor.GetPublicKey()
	if err != nil {
		return nil, xerrors.Errorf("failed to get the DKG public key: %v", err)
	}

	return pubKey, nil
}

// Write implements lottery.Secret
func (c *Calypso) Write(em lottery.EncryptedMessage,
	ac arc.AccessControl) ([]byte, error) {

	var buf bytes.Buffer

	_, err := em.GetK().MarshalTo(&buf)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal K: %v", err)
	}

	_, err = em.GetC().MarshalTo(&buf)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal C: %v", err)
	}

	key := crypto.NewSha256Factory().New().Sum(buf.Bytes())

	record := record{
		K:  em.GetK(),
		C:  em.GetC(),
		AC: ac,
	}

	c.storage.Store(key, record)

	return key, nil
}

// Read implements lottery.Secret
func (c *Calypso) Read(id []byte, idents ...arc.Identity) ([]byte, error) {
	record, err := c.getRead(id)
	if err != nil {
		return nil, xerrors.Errorf("failed to get read: %v", err)
	}

	err = record.AC.Match(ArcRuleRead, idents...)
	if err != nil {
		return nil, xerrors.Errorf("darc verification failed: %v", err)
	}

	msg, err := c.dkgActor.Decrypt(record.K, record.C)
	if err != nil {
		return nil, xerrors.Errorf("failed to decrypt with dkg: %v", err)
	}

	return msg, nil
}

// UpdateAccess implements lottery.Secret. It sets a new arc for a given ID,
// provided the current arc allows the given ident to do so.
func (c *Calypso) UpdateAccess(id []byte, ident arc.Identity,
	newAc arc.AccessControl) error {

	record, err := c.getRead(id)
	if err != nil {
		return xerrors.Errorf("failed to get read: %v", err)
	}

	err = record.AC.Match(ArcRuleUpdate, ident)
	if err != nil {
		return xerrors.Errorf("darc verification failed: %v", err)
	}

	record.AC = newAc

	c.storage.Store(id, record)

	return nil
}

// getRead extract the read information from the storage
func (c *Calypso) getRead(id []byte) (*record, error) {
	buf, err := c.storage.Read(id)
	if err != nil {
		return nil, xerrors.Errorf("failed to read message: %v", err)
	}

	var record record
	err = serdej.NewSerializer().Deserialize(buf, NewRecordFactory(), &record)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal JSON message: %v", err)
	}

	return &record, nil
}

// record defines what is stored in the db, which is the secrect and its
// corresponding access control
type record struct {
	serde.UnimplementedMessage

	K  kyber.Point
	C  kyber.Point
	AC arc.AccessControl
}

// VisitJSON implements serde.Message. It returns the JSON message for the
// record.
func (w record) VisitJSON(ser serde.Serializer) (interface{}, error) {
	acBuf, err := ser.Serialize(w.AC)
	if err != nil {
		return nil, xerrors.Errorf("failed to serialize the access control: %v", err)
	}

	kBuf, err := w.K.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal K: %v", err)
	}

	cBuf, err := w.C.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal C: %v", err)
	}

	return json.Record{
		K:  kBuf,
		C:  cBuf,
		AC: acBuf,
	}, nil
}

// recordFactory is a factory to instantiate a record from its encoded form
//
// - implements serde.Factory
type recordFactory struct {
	serde.UnimplementedFactory

	darcFactory serde.Factory
}

// NewRecordFactory returns a new instance of the record factory.
func NewRecordFactory() serde.Factory {
	return recordFactory{
		darcFactory: darc.NewFactory(),
	}
}

// VisitJSON implements serde.Factory. It deserializes the record.
func (f recordFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Record{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize task: %v", err)
	}

	var ac arc.AccessControl
	err = in.GetSerializer().Deserialize(m.AC, f.darcFactory, &ac)
	if err != nil {
		return nil, xerrors.Errorf("failed to deserialize the access control: %v", err)
	}

	K := suite.Point()
	err = K.UnmarshalBinary(m.K)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal K: %v", err)
	}

	C := suite.Point()
	err = C.UnmarshalBinary(m.C)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal C: %v", err)
	}

	return record{
		K:  K,
		C:  C,
		AC: ac,
	}, nil
}
