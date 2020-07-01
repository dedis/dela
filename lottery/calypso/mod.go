package calypso

import (
	"crypto/rand"
	"encoding/json"

	"go.dedis.ch/dela/serde"
	serdej "go.dedis.ch/dela/serde/json"

	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/ledger/arc"
	"go.dedis.ch/dela/ledger/arc/darc"
	"go.dedis.ch/dela/lottery"
	"go.dedis.ch/dela/lottery/storage"
	"go.dedis.ch/dela/lottery/storage/inmemory"
	"go.dedis.ch/dela/mino"
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
		dkg:        dkg,
		storage:    inmemory.NewInMemory(),
		serializer: serdej.NewSerializer(),
	}
}

// Calypso is a wrapper around DKG to provide a storage and authorization layer
//
// implements lottery.Secret
type Calypso struct {
	dkg        dkg.DKG
	dkgActor   dkg.Actor
	storage    storage.KeyValue
	serializer serde.Serializer
}

// Setup implements lottery.Secret
func (c *Calypso) Setup(players mino.Players, pubKeys []kyber.Point,
	threshold int) (pubKey kyber.Point, err error) {

	actor, err := c.dkg.Listen()
	if err != nil {
		return nil, xerrors.Errorf("failed to listen dkg: %v", err)
	}

	pubKey, err = actor.Setup(players, pubKeys, threshold)
	if err != nil {
		return nil, xerrors.Errorf("failed to setup dkg: %v", err)
	}

	c.dkgActor = actor

	return pubKey, nil
}

// Write implements lottery.Secret
func (c *Calypso) Write(message lottery.EncryptedMessage, ac arc.AccessControl) ([]byte, error) {
	key := make([]byte, keySize)
	_, err := rand.Read(key)
	if err != nil {
		return nil, xerrors.Errorf("failed to generate random key: %v", err)
	}

	darcKey := make([]byte, keySize)
	_, err = rand.Read(darcKey)
	if err != nil {
		return nil, xerrors.Errorf("failed to generate random darcKey: %v", err)
	}

	kBuf, err := message.GetK().MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal K: %v", err)
	}

	cBuf, err := message.GetC().MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal C: %v", err)
	}

	acBuf, err := c.serializer.Serialize(ac)
	if err != nil {
		return nil, xerrors.Errorf("failed to encode darc: %v", err)
	}

	messageJSON := encryptedJSON{
		K:       kBuf,
		C:       cBuf,
		DarcKey: darcKey,
	}

	messageBuf, err := json.Marshal(messageJSON)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal message: %v", err)
	}

	c.storage.Store(key, messageBuf)
	c.storage.Store(darcKey, acBuf)

	return key, nil
}

// Read implements lottery.Secret
func (c *Calypso) Read(id []byte, idents ...arc.Identity) ([]byte, error) {
	messageJSON, ac, err := c.getRead(id)
	if err != nil {
		return nil, xerrors.Errorf("failed to get read: %v", err)
	}

	err = ac.Match(ArcRuleRead, idents...)
	if err != nil {
		return nil, xerrors.Errorf("darc verification failed: %v", err)
	}

	k := suite.Point()
	err = k.UnmarshalBinary(messageJSON.K)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal k: %v", err)
	}

	cp := suite.Point() // 'c' is already used...
	err = cp.UnmarshalBinary(messageJSON.C)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal c: %v", err)
	}

	msg, err := c.dkgActor.Decrypt(k, cp)
	if err != nil {
		return nil, xerrors.Errorf("failed to decrypt with dkg: %v", err)
	}

	return msg, nil
}

// UpdateAccess implements lottery.Secret. It sets a new arc for a given ID,
// provided the current arc allows the given ident to do so.
func (c *Calypso) UpdateAccess(id []byte, ident arc.Identity, newAc arc.AccessControl) error {

	messageJSON, ac, err := c.getRead(id)
	if err != nil {
		return xerrors.Errorf("failed to get read: %v", err)
	}

	err = ac.Match(ArcRuleUpdate, ident)
	if err != nil {
		return xerrors.Errorf("darc verification failed: %v", err)
	}

	acBuf, err := c.serializer.Serialize(newAc)
	if err != nil {
		return xerrors.Errorf("failed to encode darc: %v", err)
	}

	c.storage.Store(messageJSON.DarcKey, acBuf)

	return nil
}

// getRead extract the read information from the storage
func (c *Calypso) getRead(id []byte) (*encryptedJSON, arc.AccessControl, error) {
	messageBuf, err := c.storage.Read(id)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to read message: %v", err)
	}

	var messageJSON encryptedJSON
	err = json.Unmarshal(messageBuf, &messageJSON)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to unmarshal JSON message: %v", err)
	}

	acBuf, err := c.storage.Read(messageJSON.DarcKey)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to read darcBuf: %v", err)
	}

	var ac arc.AccessControl
	err = c.serializer.Deserialize(acBuf, darc.NewFactory(), &ac)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to unmarshal darc: %v", err)
	}

	return &messageJSON, ac, nil
}

// NewEncryptedMessage creates a new EncryptedMessage
func NewEncryptedMessage(K, C kyber.Point) EncryptedMessage {
	return EncryptedMessage{
		k: K,
		c: C,
	}
}

// EncryptedMessage defines an encrypted message
//
// implements lottery.EncryptedMessage
type EncryptedMessage struct {
	k kyber.Point
	c kyber.Point
}

// GetK implements lottery.EncryptedMessage. It returns the ephemeral DH public
// key
func (e EncryptedMessage) GetK() kyber.Point {
	return e.k
}

// GetC implements lottery.EncryptedMessage. It returns the message blinded with
// secret
func (e EncryptedMessage) GetC() kyber.Point {
	return e.c
}

// encryptedJSON is used to marshal a lottery.Encrypted message. The k and c
// should be the marshalled binairy representation of the k,c kyber.Point, and
// the DarcKey is the key at which the darc controlling this encrypted message
// is stored.
type encryptedJSON struct {
	K       []byte
	C       []byte
	DarcKey []byte
}
