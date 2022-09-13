package command

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestNewSignerAction(t *testing.T) {
	action := action{
		printer:   io.Discard,
		genSigner: badGenSigner,
		saveFile:  fakeSaveFile,
		getPubKey: getPubkey,
	}

	set := node.FlagSet{}
	err := action.newSignerAction(set)
	require.EqualError(t, err, fake.Err("failed to marshal signer"))

	action.genSigner = bls.NewSigner().MarshalBinary
	err = action.newSignerAction(set)
	require.NoError(t, err)

	set["save"] = "/do/not/exist"
	action.saveFile = badSaveFile

	err = action.newSignerAction(set)
	require.EqualError(t, err, fake.Err("failed to save files"))
}

func TestLoadSignerAction(t *testing.T) {
	action := action{
		printer:  io.Discard,
		readFile: badReadFile,
	}

	set := node.FlagSet{}
	err := action.loadSignerAction(set)
	require.EqualError(t, err, fake.Err("failed to read data"))

	action.readFile = fakeReadFile
	err = action.loadSignerAction(set)
	require.EqualError(t, err, "unknown format ''")

	set["format"] = "PUBKEY"
	action.getPubKey = badGetPubKey
	err = action.loadSignerAction(set)
	require.EqualError(t, err, fake.Err("failed to get PUBKEY"))

	action.getPubKey = wrongGetPubKey
	err = action.loadSignerAction(set)
	require.EqualError(t, err, fake.Err("failed to marshal pubkey"))

	set["format"] = "BASE64_PUBKEY"
	action.getPubKey = badGetPubKey
	err = action.loadSignerAction(set)
	require.EqualError(t, err, fake.Err("failed to get PUBKEY"))

	action.getPubKey = wrongGetPubKey
	err = action.loadSignerAction(set)
	require.EqualError(t, err, fake.Err("failed to marshal pubkey"))

	set["format"] = "BASE64_PUBKEY"
	action.getPubKey = fakeGetPubKey
	err = action.loadSignerAction(set)
	require.NoError(t, err)

	set["format"] = "BASE64"
	action.getPubKey = badGetPubKey
	err = action.loadSignerAction(set)
	require.NoError(t, err)
}

func TestSaveToFile(t *testing.T) {
	path, err := os.MkdirTemp("", "dela-test-")
	require.NoError(t, err)

	defer os.RemoveAll(path)

	file := filepath.Join(path, "test")
	err = saveToFile(file, false, []byte{1})
	require.NoError(t, err)

	res, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, []byte{1}, res)

	err = saveToFile(file, false, nil)
	require.Regexp(t, "^file '.*' already exist, use --force if you want to overwrite$", err)

	err = saveToFile("/not/exist", true, nil)
	require.Regexp(t, "^failed to write file:", err)

	err = saveToFile(file, true, []byte{2})
	require.NoError(t, err)

	res, err = os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, []byte{2}, res)
}

func TestGetPUBKEY_Happy(t *testing.T) {
	buf, err := bls.NewSigner().MarshalBinary()
	require.NoError(t, err)

	_, err = getPubkey(buf)
	require.NoError(t, err)
}

func TestGetPUBKEY_Error(t *testing.T) {
	_, err := getPubkey(nil)
	require.EqualError(t, err, "failed to unmarshal signer: while unmarshaling scalar: UnmarshalBinary: wrong size buffer")
}

// -----------------------------------------------------------------------------
// Utility functions

func badGenSigner() ([]byte, error) {
	return nil, fake.GetError()
}

func badReadFile(path string) ([]byte, error) {
	return nil, fake.GetError()
}

func badSaveFile(path string, force bool, data []byte) error {
	return fake.GetError()
}

func fakeReadFile(path string) ([]byte, error) {
	return nil, nil
}

func fakeSaveFile(path string, force bool, data []byte) error {
	return nil
}

func badGetPubKey([]byte) (crypto.PublicKey, error) {
	return nil, fake.GetError()
}

func wrongGetPubKey([]byte) (crypto.PublicKey, error) {
	return fake.NewBadPublicKey(), nil
}

func fakeGetPubKey([]byte) (crypto.PublicKey, error) {
	return bls.Generate().GetPublicKey(), nil
}
