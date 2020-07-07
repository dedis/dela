package calypso

import (
	"bytes"

	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"

	"go.dedis.ch/dela/calypso/storage"
	"go.dedis.ch/dela/calypso/storage/inmemory"
	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/ledger/arc"
	"go.dedis.ch/dela/ledger/arc/darc"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
)

const (
	// ArcRuleUpdate defines the rule to update the arc. This rule must be set
	// at the write creation to allow the arc to be latter updated.
	ArcRuleUpdate = "calypso_update"
	// ArcRuleRead defines the arc rule to read a value
	ArcRuleRead = "calypso_read"
)

// Suite is the Kyber suite for Pedersen.
var suite = suites.MustFind("Ed25519")

var recordFormats = registry.NewSimpleRegistry()

// RegisterRecordFormats registers the engine for the provided format.
func RegisterRecordFormats(format serde.Format, engine serde.FormatEngine) {
	recordFormats.Register(format, engine)
}

// caly is a wrapper around DKG that provides a private storage
//
// implements calypso.PrivateStorage
type caly struct {
	dkgActor dkg.Actor
	storage  storage.KeyValue
}

// newCalypso creates a new Calypso
func newCalypso(actor dkg.Actor) *caly {
	return &caly{
		dkgActor: actor,
		storage:  inmemory.NewInMemory(),
	}
}

// Setup implements calypso.PrivateStorage
func (c *caly) Setup(ca crypto.CollectiveAuthority,
	threshold int) (pubKey kyber.Point, err error) {

	pubKey, err = c.dkgActor.Setup(ca, threshold)
	if err != nil {
		return nil, xerrors.Errorf("failed to setup: %v", err)
	}

	return pubKey, nil
}

// GetPublicKey implements calypso.PrivateStorage
func (c *caly) GetPublicKey() (kyber.Point, error) {
	if c.dkgActor == nil {
		return nil, xerrors.Errorf("listen has not already been called")
	}

	pubKey, err := c.dkgActor.GetPublicKey()
	if err != nil {
		return nil, xerrors.Errorf("failed to get the DKG public key: %v", err)
	}

	return pubKey, nil
}

// Write implements calypso.PrivateStorage
func (c *caly) Write(em EncryptedMessage,
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

	hash := crypto.NewSha256Factory().New()
	_, err = hash.Write(buf.Bytes())
	if err != nil {
		return nil, xerrors.Errorf("failed to compute hash: %v", err)
	}

	key := hash.Sum(nil)

	record := Record{
		k:      em.GetK(),
		c:      em.GetC(),
		access: ac,
	}

	c.storage.Store(key, record)

	return key, nil
}

// Read implements calypso.PrivateStorage
func (c *caly) Read(id []byte, idents ...arc.Identity) ([]byte, error) {
	record, err := c.getRead(id)
	if err != nil {
		return nil, xerrors.Errorf("failed to get read: %v", err)
	}

	err = record.access.Match(ArcRuleRead, idents...)
	if err != nil {
		return nil, xerrors.Errorf("darc verification failed: %v", err)
	}

	msg, err := c.dkgActor.Decrypt(record.k, record.c)
	if err != nil {
		return nil, xerrors.Errorf("failed to decrypt with dkg: %v", err)
	}

	return msg, nil
}

// UpdateAccess implements calypso.PrivateStorage. It sets a new arc for a given
// ID, provided the current arc allows the given ident to do so.
func (c *caly) UpdateAccess(id []byte, ident arc.Identity,
	newAc arc.AccessControl) error {

	record, err := c.getRead(id)
	if err != nil {
		return xerrors.Errorf("failed to get read: %v", err)
	}

	err = record.access.Match(ArcRuleUpdate, ident)
	if err != nil {
		return xerrors.Errorf("darc verification failed: %v", err)
	}

	record.access = newAc

	c.storage.Store(id, record)

	return nil
}

// getRead extract the read information from the storage
func (c *caly) getRead(id []byte) (Record, error) {
	message, err := c.storage.Read(id)
	if err != nil {
		return Record{}, xerrors.Errorf("failed to read message: %v", err)
	}

	record, ok := message.(Record)
	if !ok {
		return Record{}, xerrors.Errorf("expected to find '%T' but found '%T'", record, message)
	}

	return record, nil
}

// Record defines what is stored in the db, which is the secrect and its
// corresponding access control
type Record struct {
	k      kyber.Point
	c      kyber.Point
	access arc.AccessControl
}

// NewRecord creates a new record from the points and the access control.
func NewRecord(K, C kyber.Point, access arc.AccessControl) Record {
	return Record{
		k:      K,
		c:      C,
		access: access,
	}
}

// GetK returns K.
func (r Record) GetK() kyber.Point {
	return r.k
}

// GetC returns C.
func (r Record) GetC() kyber.Point {
	return r.c
}

// GetAccess returns the access control for this record.
func (r Record) GetAccess() arc.AccessControl {
	return r.access
}

// Serialize implements serde.Message.
func (r Record) Serialize(ctx serde.Context) ([]byte, error) {
	format := recordFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, r)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// AccessKeyFac is the key to the access control factory.
type AccessKeyFac struct{}

// recordFactory is a factory to instantiate a record from its encoded form
//
// - implements serde.Factory
type recordFactory struct {
	darcFactory arc.AccessControlFactory
}

// NewRecordFactory returns a new instance of the record factory.
func NewRecordFactory() serde.Factory {
	return recordFactory{
		darcFactory: darc.NewFactory(),
	}
}

// VisitJSON implements serde.Factory. It deserializes the record.
func (f recordFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := recordFormats.Get(ctx.GetFormat())

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
