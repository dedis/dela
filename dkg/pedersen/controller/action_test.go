package controller

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/kyber/v3"
)

func TestSetupAction_NoActor(t *testing.T) {
	a := setupAction{}

	inj := node.NewInjector()

	ctx := node.Context{
		Injector: inj,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, "failed to resolve actor, did you call listen?: "+
		"couldn't find dependency for 'dkg.Actor'")
}

func TestSetupAction_NoCollectiveAuth(t *testing.T) {
	a := setupAction{}

	inj := node.NewInjector()
	inj.Inject(&fakeActor{})

	flags := node.FlagSet{
		"authority": []interface{}{"fake"},
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, "failed to get collective authority: "+
		"failed to decode authority: invalid identity base64 string")
}

func TestSetupAction_badSetup(t *testing.T) {
	a := setupAction{}

	inj := node.NewInjector()
	inj.Inject(&fakeActor{
		setupErr: fake.GetError(),
	})

	flags := node.FlagSet{}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, fake.Err("failed to setup"))
}

func TestSetupAction_OK(t *testing.T) {
	a := setupAction{}

	inj := node.NewInjector()
	inj.Inject(&fakeActor{})

	flags := node.FlagSet{}

	out := &bytes.Buffer{}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
		Out:      out,
	}

	err := a.Execute(ctx)
	require.NoError(t, err)

	require.Regexp(t, "^✅ Setup done", out.String())
}

func TestGetCollectiveAuth(t *testing.T) {
	pubKey := "RjEyNy4wLjAuMToyMDAx:RcTqbSXCIkZmmaGjLVAZs8TdTvq7b3SFr14F89h6ID8="

	inj := node.NewInjector()
	inj.Inject(fake.Mino{})

	flags := node.FlagSet{
		"authority": []interface{}{pubKey},
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	res, err := getCollectiveAuth(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, res.Len())
}

func TestListenAction_NoDKG(t *testing.T) {
	a := listenAction{}

	inj := node.NewInjector()

	ctx := node.Context{
		Injector: inj,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, "failed to resolve dkg: couldn't find dependency for 'dkg.DKG'")
}

func TestListenAction_listenFail(t *testing.T) {
	a := listenAction{}

	inj := node.NewInjector()
	inj.Inject(fakeDKG{
		err: fake.GetError(),
	})

	ctx := node.Context{
		Injector: inj,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, fake.Err("failed to listen"))
}

func TestListenAction_encodeFail(t *testing.T) {
	a := listenAction{}

	inj := node.NewInjector()
	inj.Inject(fakeDKG{
		actor: fakeActor{},
	})

	ctx := node.Context{
		Injector: inj,
		Out:      io.Discard,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, "failed to encode authority: failed to resolve"+
		" mino: couldn't find dependency for 'mino.Mino'")
}

func TestListenAction_writeFail(t *testing.T) {
	a := listenAction{
		pubkey: suite.Point(),
	}

	inj := node.NewInjector()
	inj.Inject(fakeDKG{
		actor: fakeActor{},
	})
	inj.Inject(fake.Mino{})

	flags := node.FlagSet{
		"config": "/fake/fake", // wrong configuration path
	}

	ctx := node.Context{
		Injector: inj,
		Out:      io.Discard,
		Flags:    flags,
	}

	err := a.Execute(ctx)
	require.Regexp(t, "^failed to write authority configuration", err.Error())
}

func TestListenAction_OK(t *testing.T) {
	tmpDir := t.TempDir()

	a := listenAction{
		pubkey: suite.Point(),
	}

	inj := node.NewInjector()
	inj.Inject(fakeDKG{
		actor: fakeActor{},
	})
	inj.Inject(fake.Mino{})

	flags := node.FlagSet{
		"config": tmpDir,
	}

	out := &bytes.Buffer{}

	ctx := node.Context{
		Injector: inj,
		Out:      out,
		Flags:    flags,
	}

	err := a.Execute(ctx)
	require.NoError(t, err)

	require.Regexp(t, "^✅  Listen done, actor is created.📜 Config file written in", out.String())
}

func TestEncodeAuthority_marshalFail(t *testing.T) {
	inj := node.NewInjector()
	inj.Inject(fakeDKG{
		actor: fakeActor{},
	})
	inj.Inject(fakeMino{
		addr: fake.NewBadAddress(),
	})

	ctx := node.Context{
		Injector: inj,
	}

	_, err := encodeAuthority(ctx, badPoint{})
	require.EqualError(t, err, fake.Err("failed to marshal address"))
}

func TestEncodeAuthority_pkFail(t *testing.T) {
	inj := node.NewInjector()
	inj.Inject(fakeDKG{
		actor: fakeActor{},
	})
	inj.Inject(fakeMino{
		addr: fake.NewAddress(0),
	})

	ctx := node.Context{
		Injector: inj,
	}

	pk := badPoint{
		err: fake.GetError(),
	}

	_, err := encodeAuthority(ctx, pk)
	require.EqualError(t, err, fake.Err("failed to marshall pubkey"))
}

func TestDecodeAuthority_noMino(t *testing.T) {
	pubKey := "RjEyNy4wLjAuMToyMDAx:RcTqbSXCIkZmmaGjLVAZs8TdTvq7b3SFr14F89h6ID8="

	inj := node.NewInjector()

	ctx := node.Context{
		Injector: inj,
	}

	_, _, err := decodeAuthority(ctx, pubKey)
	require.EqualError(t, err, "injector: couldn't find dependency for 'mino.Mino'")
}

func TestDecodeAuthority_noBase64Addr(t *testing.T) {
	pubKey := "aa:RcTqbSXCIkZmmaGjLVAZs8TdTvq7b3SFr14F89h6ID8="

	inj := node.NewInjector()
	inj.Inject(fake.Mino{})

	ctx := node.Context{
		Injector: inj,
	}

	_, _, err := decodeAuthority(ctx, pubKey)
	require.EqualError(t, err, "base64 address: illegal base64 data at input byte 0")
}

func TestDecodeAuthority_noBase64Pubkey(t *testing.T) {
	pubKey := "RjEyNy4wLjAuMToyMDAx:aa"

	inj := node.NewInjector()
	inj.Inject(fake.Mino{})

	ctx := node.Context{
		Injector: inj,
	}

	_, _, err := decodeAuthority(ctx, pubKey)
	require.EqualError(t, err, "base64 public key: illegal base64 data at input byte 0")
}

func TestDecodeAuthority_badUnmarshalPubkey(t *testing.T) {
	pubKey := "RjEyNy4wLjAuMToyMDAx:aaa="

	inj := node.NewInjector()
	inj.Inject(fake.Mino{})

	ctx := node.Context{
		Injector: inj,
	}

	_, _, err := decodeAuthority(ctx, pubKey)
	require.EqualError(t, err, "failed to decode pubkey: invalid Ed25519 curve point")
}

func TestEncryptAction_noActor(t *testing.T) {
	a := encryptAction{}

	inj := node.NewInjector()

	ctx := node.Context{
		Injector: inj,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, "failed to resolve actor, did you call listen?: "+
		"couldn't find dependency for 'dkg.Actor'")
}

func TestEncryptAction_badMessage(t *testing.T) {
	a := encryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{})

	flags := node.FlagSet{
		"message": "not hex",
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err := a.Execute(ctx)
	require.Regexp(t, "^failed to decode message:", err.Error())
}

func TestEncryptAction_encryptFail(t *testing.T) {
	a := encryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{
		encryptErr: fake.GetError(),
	})

	flags := node.FlagSet{
		"message": "aef123",
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, fake.Err("failed to encrypt"))
}

func TestEncryptAction_encodeFail(t *testing.T) {
	a := encryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{
		k: badPoint{
			err: fake.GetError(),
		},
	})

	flags := node.FlagSet{
		"message": "aef123",
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, fake.Err("failed to generate output: failed to marshal k"))
}

// TODO jean: re-enable this test ASAP
func IgnoreTestEncryptAction_OK(t *testing.T) {
	a := encryptAction{}

	data := "aa"
	fakeK := badPoint{data: data}

	var array interface{} = badPointArray{data: data}
	fakeCs := array.([]kyber.Point)

	//	fakeCs := make([]badPoint, 1)
	//	fakeCs = append(fakeCs, badPoint{data: data})
	actor := fakeActor{k: fakeK, cs: fakeCs}

	inj := node.NewInjector()
	inj.Inject(actor)

	flags := node.FlagSet{
		"message": "aef123",
	}

	out := &bytes.Buffer{}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
		Out:      out,
	}

	err := a.Execute(ctx)
	require.NoError(t, err)

	dataHex := hex.EncodeToString([]byte(data))

	require.Equal(t, dataHex+":"+dataHex, out.String())
}

func TestDecryptAction_noActor(t *testing.T) {
	a := decryptAction{}

	inj := node.NewInjector()

	ctx := node.Context{
		Injector: inj,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, "failed to resolve actor, did you call listen?:"+
		" couldn't find dependency for 'dkg.Actor'")
}

func TestDecryptAction_decodeFail(t *testing.T) {
	a := decryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{})

	flags := node.FlagSet{
		"encrypted": "not hex",
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, "failed to decode encrypted str: malformed encoded: not hex")
}

func TestDecryptAction_decryptFail(t *testing.T) {
	encrypted := "abea449f0ab86029c529f541cdd7f48aee3102b9c1ea2999b5d39c2cc49a4c23:ae29dd65cb4ceaaf7830008b9544625a05b6dbbcd421cf8475aedbef8e8d1da9"
	a := decryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{
		decryptErr: fake.GetError(),
	})

	flags := node.FlagSet{
		"encrypted": encrypted,
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, fake.Err("failed to decrypt"))
}

func TestDecryptAction_decryptOK(t *testing.T) {
	encrypted := "abea449f0ab86029c529f541cdd7f48aee3102b9c1ea2999b5d39c2cc49a4c23:ae29dd65cb4ceaaf7830008b9544625a05b6dbbcd421cf8475aedbef8e8d1da9"

	message := "fake"
	expected := hex.EncodeToString([]byte(message))

	a := decryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{
		decryptData: []byte(message),
	})

	flags := node.FlagSet{
		"encrypted": encrypted,
	}

	out := &bytes.Buffer{}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
		Out:      out,
	}

	err := a.Execute(ctx)
	require.NoError(t, err)

	require.Equal(t, expected, out.String())
}

func TestEncodeEncrypted_marshalKFail(t *testing.T) {
	_, err := encodeEncrypted(badPoint{err: fake.GetError()}, nil)
	require.EqualError(t, err, fake.Err("failed to marshal k"))
}

// TODO jean: reenable this test
func IgnoreTestEncodeEncrypted_marshalCSFail(t *testing.T) {
	//_, err := encodeEncrypted(badPoint{}, badPointArray{
	// cs: make([]kyber.Point, 1), err: fake.GetError()})
	//require.EqualError(t, err, fake.Err("failed to marshal c"))
}

func TestDecodeEncrypted_kHexBad(t *testing.T) {
	encrypted := "fake:fake:"

	_, _, err := decodeEncrypted(encrypted)
	require.Regexp(t, "^failed to decode k point", err.Error())
}

func TestDecodeEncrypted_kUnmarshalBad(t *testing.T) {
	encrypted := "aef123:fake:"

	_, _, err := decodeEncrypted(encrypted)
	require.EqualError(t, err, "failed to unmarshal k point: invalid Ed25519 curve point")
}

func TestDecodeEncrypted_cHexBad(t *testing.T) {
	encrypted := "abea449f0ab86029c529f541cdd7f48aee3102b9c1ea2999b5d39c2cc49a4c23:fake:"

	_, _, err := decodeEncrypted(encrypted)
	require.Regexp(t, "^failed to decode c point", err.Error())
}

func TestDecodeEncrypted_cUnmarshalBad(t *testing.T) {
	encrypted := "abea449f0ab86029c529f541cdd7f48aee3102b9c1ea2999b5d39c2cc49a4c23:aef123:"

	_, _, err := decodeEncrypted(encrypted)
	require.EqualError(t, err, "failed to unmarshal c point: invalid Ed25519 curve point")
}

func TestVerifiableEncryptAction_noActor(t *testing.T) {
	a := verifiableEncryptAction{}

	inj := node.NewInjector()

	ctx := node.Context{
		Injector: inj,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, "failed to resolve actor, did you call listen?: couldn't find dependency for 'dkg.Actor'")
}

func TestVerifiableEncryptAction_badHexGbar(t *testing.T) {
	a := verifiableEncryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{})

	flags := node.FlagSet{
		"GBar": "not hex",
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err := a.Execute(ctx)
	require.Regexp(t, "^failed to decode GBar point", err.Error())
}

func TestVerifiableEncryptAction_GbarUnmarshalFail(t *testing.T) {
	a := verifiableEncryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{})

	flags := node.FlagSet{
		"GBar": "aef123",
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, "failed to unmarshal GBar point: invalid Ed25519 curve point")
}

func TestVerifiableEncryptAction_decodeMessageFail(t *testing.T) {
	gbar := suite.Point()

	gbarBuf, err := gbar.MarshalBinary()
	require.NoError(t, err)

	a := verifiableEncryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{})

	flags := node.FlagSet{
		"GBar":    hex.EncodeToString(gbarBuf),
		"message": "not hex",
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.Regexp(t, "^failed to decode message", err.Error())
}

func TestVerifiableEncryptAction_decryptFail(t *testing.T) {
	gbar := suite.Point()

	gbarBuf, err := gbar.MarshalBinary()
	require.NoError(t, err)

	a := verifiableEncryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{
		vencryptErr: fake.GetError(),
	})

	flags := node.FlagSet{
		"GBar":    hex.EncodeToString(gbarBuf),
		"message": "aef123",
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.EqualError(t, err, fake.Err("failed to encrypt"))
}

func TestVerifiableEncryptAction_encodeKFail(t *testing.T) {
	gbar := suite.Point()

	gbarBuf, err := gbar.MarshalBinary()
	require.NoError(t, err)

	a := verifiableEncryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{
		ct: types.Ciphertext{
			K: badPoint{
				err: fake.GetError(),
			},
		},
	})

	flags := node.FlagSet{
		"GBar":    hex.EncodeToString(gbarBuf),
		"message": "aef123",
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.EqualError(t, err, fake.Err("failed to marshal k"))
}

func TestVerifiableEncryptAction_encodeCFail(t *testing.T) {
	gbar := suite.Point()

	gbarBuf, err := gbar.MarshalBinary()
	require.NoError(t, err)

	a := verifiableEncryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{
		ct: types.Ciphertext{
			K: badPoint{},
			C: badPoint{
				err: fake.GetError(),
			},
		},
	})

	flags := node.FlagSet{
		"GBar":    hex.EncodeToString(gbarBuf),
		"message": "aef123",
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.EqualError(t, err, fake.Err("failed to marshal c"))
}

func TestVerifiableEncryptAction_encodeUbarFail(t *testing.T) {
	gbar := suite.Point()

	gbarBuf, err := gbar.MarshalBinary()
	require.NoError(t, err)

	a := verifiableEncryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{
		ct: types.Ciphertext{
			K: badPoint{},
			C: badPoint{},
			UBar: badPoint{
				err: fake.GetError(),
			},
		},
	})

	flags := node.FlagSet{
		"GBar":    hex.EncodeToString(gbarBuf),
		"message": "aef123",
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.EqualError(t, err, fake.Err("failed to marshal Ubar"))
}

func TestVerifiableEncryptAction_encodeEFail(t *testing.T) {
	gbar := suite.Point()

	gbarBuf, err := gbar.MarshalBinary()
	require.NoError(t, err)

	a := verifiableEncryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{
		ct: types.Ciphertext{
			K:    badPoint{},
			C:    badPoint{},
			UBar: badPoint{},
			E: badScalar{
				err: fake.GetError(),
			},
		},
	})

	flags := node.FlagSet{
		"GBar":    hex.EncodeToString(gbarBuf),
		"message": "aef123",
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.EqualError(t, err, fake.Err("failed to marshal E"))
}

func TestVerifiableEncryptAction_encodeFFail(t *testing.T) {
	gbar := suite.Point()

	gbarBuf, err := gbar.MarshalBinary()
	require.NoError(t, err)

	a := verifiableEncryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{
		ct: types.Ciphertext{
			K:    badPoint{},
			C:    badPoint{},
			UBar: badPoint{},
			E:    badScalar{},
			F: badScalar{
				err: fake.GetError(),
			},
		},
	})

	flags := node.FlagSet{
		"GBar":    hex.EncodeToString(gbarBuf),
		"message": "aef123",
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.EqualError(t, err, fake.Err("failed to marshal F"))
}

func TestVerifiableEncryptAction_OK(t *testing.T) {
	gbar := suite.Point()

	gbarBuf, err := gbar.MarshalBinary()
	require.NoError(t, err)

	a := verifiableEncryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{
		ct: types.Ciphertext{
			K:    badPoint{},
			C:    badPoint{},
			UBar: badPoint{},
			E:    badScalar{},
			F:    badScalar{},
		},
	})

	flags := node.FlagSet{
		"GBar":    hex.EncodeToString(gbarBuf),
		"message": "aef123",
	}

	out := &bytes.Buffer{}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
		Out:      out,
	}

	err = a.Execute(ctx)
	require.NoError(t, err)

	// empty data, we expect only the separators
	require.Equal(t, ":::::", out.String())
}

func TestVerifiableDecryptAction_noActor(t *testing.T) {
	a := verifiableDecryptAction{}

	inj := node.NewInjector()

	ctx := node.Context{
		Injector: inj,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, "failed to resolve actor, did you call listen?: couldn't find dependency for 'dkg.Actor'")
}

func TestVerifiableDecryptAction_badHexGbar(t *testing.T) {
	a := verifiableDecryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{})

	flags := node.FlagSet{
		"GBar": "not hex",
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err := a.Execute(ctx)
	require.Regexp(t, "^failed to decode GBar point", err.Error())
}

func TestVerifiableDecryptAction_GbarUnmarshalFail(t *testing.T) {
	a := verifiableDecryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{})

	flags := node.FlagSet{
		"GBar": "aef123",
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, "failed to unmarshal GBar point: invalid Ed25519 curve point")
}

func TestVerifiableDecryptAction_ciphertextSplitFail(t *testing.T) {
	gbar := suite.Point()

	gbarBuf, err := gbar.MarshalBinary()
	require.NoError(t, err)

	a := verifiableDecryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{})

	flags := node.FlagSet{
		"GBar":        hex.EncodeToString(gbarBuf),
		"ciphertexts": "::",
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.EqualError(t, err, "malformed encoded: ::")
}

func TestVerifiableDecryptAction_badHexK(t *testing.T) {
	ciphertext := "not hex::::"

	gbar := suite.Point()

	gbarBuf, err := gbar.MarshalBinary()
	require.NoError(t, err)

	a := verifiableDecryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{})

	flags := node.FlagSet{
		"GBar":        hex.EncodeToString(gbarBuf),
		"ciphertexts": ciphertext,
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.Regexp(t, "^failed to decode k point", err.Error())
}

func TestVerifiableDecryptAction_badUnmarshalK(t *testing.T) {
	aPoint := suite.Point()

	aPointBuf, err := aPoint.MarshalBinary()
	require.NoError(t, err)

	aPointHex := hex.EncodeToString(aPointBuf)

	a := verifiableDecryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{})

	ciphertext := "aef123::::"

	flags := node.FlagSet{
		"GBar":        aPointHex,
		"ciphertexts": ciphertext,
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.EqualError(t, err, "failed to unmarshal k point: invalid Ed25519 curve point")
}

func TestVerifiableDecryptAction_badHexC(t *testing.T) {
	aPoint := suite.Point()

	aPointBuf, err := aPoint.MarshalBinary()
	require.NoError(t, err)

	aPointHex := hex.EncodeToString(aPointBuf)

	a := verifiableDecryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{})

	ciphertext := aPointHex + ":not hex:::"

	flags := node.FlagSet{
		"GBar":        aPointHex,
		"ciphertexts": ciphertext,
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.Regexp(t, "^failed to decode c point", err.Error())
}

func TestVerifiableDecryptAction_badUnmarshalC(t *testing.T) {
	aPoint := suite.Point()

	aPointBuf, err := aPoint.MarshalBinary()
	require.NoError(t, err)

	aPointHex := hex.EncodeToString(aPointBuf)

	a := verifiableDecryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{})

	ciphertext := aPointHex + ":aef123:::"

	flags := node.FlagSet{
		"GBar":        aPointHex,
		"ciphertexts": ciphertext,
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.EqualError(t, err, "failed to unmarshal c point: invalid Ed25519 curve point")
}

func TestVerifiableDecryptAction_badHexUbar(t *testing.T) {
	aPoint := suite.Point()

	aPointBuf, err := aPoint.MarshalBinary()
	require.NoError(t, err)

	aPointHex := hex.EncodeToString(aPointBuf)

	a := verifiableDecryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{})

	ciphertext := fmt.Sprintf("%s:%s:%s:%s:%s", aPointHex, aPointHex, "not hex", "", "")

	flags := node.FlagSet{
		"GBar":        aPointHex,
		"ciphertexts": ciphertext,
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.Regexp(t, "^failed to decode UBar point", err.Error())
}

func TestVerifiableDecryptAction_badUnmarshalUbar(t *testing.T) {
	aPoint := suite.Point()

	aPointBuf, err := aPoint.MarshalBinary()
	require.NoError(t, err)

	aPointHex := hex.EncodeToString(aPointBuf)

	a := verifiableDecryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{})

	ciphertext := fmt.Sprintf("%s:%s:%s:%s:%s", aPointHex, aPointHex, "aef123", "", "")

	flags := node.FlagSet{
		"GBar":        aPointHex,
		"ciphertexts": ciphertext,
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.EqualError(t, err, "failed to unmarshal UBar point: invalid Ed25519 curve point")
}

func TestVerifiableDecryptAction_badHexE(t *testing.T) {
	aPoint := suite.Point()

	aPointBuf, err := aPoint.MarshalBinary()
	require.NoError(t, err)

	aPointHex := hex.EncodeToString(aPointBuf)

	a := verifiableDecryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{})

	ciphertext := fmt.Sprintf("%s:%s:%s:%s:%s", aPointHex, aPointHex, aPointHex, "not hex", "")

	flags := node.FlagSet{
		"GBar":        aPointHex,
		"ciphertexts": ciphertext,
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.Regexp(t, "^failed to decode E", err.Error())
}

func TestVerifiableDecryptAction_badUnmarshalE(t *testing.T) {
	aPoint := suite.Point()

	aPointBuf, err := aPoint.MarshalBinary()
	require.NoError(t, err)

	aPointHex := hex.EncodeToString(aPointBuf)

	a := verifiableDecryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{})

	ciphertext := fmt.Sprintf("%s:%s:%s:%s:%s", aPointHex, aPointHex, aPointHex, "aef123", "")

	flags := node.FlagSet{
		"GBar":        aPointHex,
		"ciphertexts": ciphertext,
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.EqualError(t, err, "failed to unmarshal E: wrong size buffer")
}

func TestVerifiableDecryptAction_badHexF(t *testing.T) {
	aPoint := suite.Point()
	aScalar := suite.Scalar()

	aPointBuf, err := aPoint.MarshalBinary()
	require.NoError(t, err)

	aScalarBuf, err := aScalar.MarshalBinary()
	require.NoError(t, err)

	aPointHex := hex.EncodeToString(aPointBuf)
	aScalarHex := hex.EncodeToString(aScalarBuf)

	a := verifiableDecryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{})

	ciphertext := fmt.Sprintf("%s:%s:%s:%s:%s", aPointHex, aPointHex, aPointHex, aScalarHex, "not hex")

	flags := node.FlagSet{
		"GBar":        aPointHex,
		"ciphertexts": ciphertext,
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.Regexp(t, "^failed to decode F", err.Error())
}

func TestVerifiableDecryptAction_badUnmarshalF(t *testing.T) {
	aPoint := suite.Point()
	aScalar := suite.Scalar()

	aPointBuf, err := aPoint.MarshalBinary()
	require.NoError(t, err)

	aScalarBuf, err := aScalar.MarshalBinary()
	require.NoError(t, err)

	aPointHex := hex.EncodeToString(aPointBuf)
	aScalarHex := hex.EncodeToString(aScalarBuf)

	a := verifiableDecryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{})

	ciphertext := fmt.Sprintf("%s:%s:%s:%s:%s", aPointHex, aPointHex, aPointHex, aScalarHex, "aef123")

	flags := node.FlagSet{
		"GBar":        aPointHex,
		"ciphertexts": ciphertext,
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.EqualError(t, err, "failed to unmarshal F: wrong size buffer")
}

func TestVerifiableDecryptAction_decryptFail(t *testing.T) {
	aPoint := suite.Point()
	aScalar := suite.Scalar()

	aPointBuf, err := aPoint.MarshalBinary()
	require.NoError(t, err)

	aScalarBuf, err := aScalar.MarshalBinary()
	require.NoError(t, err)

	aPointHex := hex.EncodeToString(aPointBuf)
	aScalarHex := hex.EncodeToString(aScalarBuf)

	a := verifiableDecryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{
		vdecryptErr: fake.GetError(),
	})

	ciphertext := fmt.Sprintf("%s:%s:%s:%s:%s", aPointHex, aPointHex, aPointHex, aScalarHex, aScalarHex)

	flags := node.FlagSet{
		"GBar":        aPointHex,
		"ciphertexts": ciphertext,
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err = a.Execute(ctx)
	require.EqualError(t, err, fake.Err("failed to decrypt"))
}

func TestVerifiableDecryptAction_OK(t *testing.T) {
	aPoint := suite.Point()
	aScalar := suite.Scalar()

	aPointBuf, err := aPoint.MarshalBinary()
	require.NoError(t, err)

	aScalarBuf, err := aScalar.MarshalBinary()
	require.NoError(t, err)

	aPointHex := hex.EncodeToString(aPointBuf)
	aScalarHex := hex.EncodeToString(aScalarBuf)

	a := verifiableDecryptAction{}

	inj := node.NewInjector()
	inj.Inject(fakeActor{
		vdecryptData: make([][]byte, 1),
	})

	ciphertext := fmt.Sprintf("%s:%s:%s:%s:%s", aPointHex, aPointHex, aPointHex, aScalarHex, aScalarHex)

	flags := node.FlagSet{
		"GBar":        aPointHex,
		"ciphertexts": ciphertext,
	}

	out := &bytes.Buffer{}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
		Out:      out,
	}

	err = a.Execute(ctx)
	require.NoError(t, err)

	require.Equal(t, "[]", out.String())
}

func TestReshareAction_noActor(t *testing.T) {
	a := reshareAction{}

	inj := node.NewInjector()

	ctx := node.Context{
		Injector: inj,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, "failed to resolve actor, did you call listen?: couldn't find dependency for 'dkg.Actor'")
}

func TestReshareAction_NoCollectiveAuth(t *testing.T) {
	a := reshareAction{}

	inj := node.NewInjector()
	inj.Inject(&fakeActor{})

	flags := node.FlagSet{
		"authority": []interface{}{"fake"},
	}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, "failed to get collective authority: "+
		"failed to decode authority: invalid identity base64 string")
}

func TestReshareAction_reshareFail(t *testing.T) {
	a := reshareAction{}

	inj := node.NewInjector()
	inj.Inject(&fakeActor{
		reshareErr: fake.GetError(),
	})

	flags := node.FlagSet{}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
	}

	err := a.Execute(ctx)
	require.EqualError(t, err, fake.Err("failed to reshare"))
}

func TestReshareAction_OK(t *testing.T) {
	a := reshareAction{}

	inj := node.NewInjector()
	inj.Inject(&fakeActor{})

	flags := node.FlagSet{}

	out := &bytes.Buffer{}

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
		Out:      out,
	}

	err := a.Execute(ctx)
	require.NoError(t, err)

	require.Equal(t, "✅ Reshare done.\n", out.String())
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeActor struct {
	dkg.Actor

	setupErr     error
	encryptErr   error
	decryptErr   error
	reencryptErr error
	vencryptErr  error
	vdecryptErr  error
	reshareErr   error

	k  kyber.Point
	cs []kyber.Point

	decryptData  []byte
	ct           types.Ciphertext
	vdecryptData [][]byte
}

func (f fakeActor) Setup(co crypto.CollectiveAuthority, threshold int) (pubKey kyber.Point, err error) {
	return suite.Point(), f.setupErr
}

func (f fakeActor) Encrypt(message []byte) (K kyber.Point, CS []kyber.Point, err error) {
	return f.k, f.cs, f.encryptErr
}

func (f fakeActor) Decrypt(K kyber.Point, CS []kyber.Point) ([]byte, error) {
	return f.decryptData, f.decryptErr
}

func (f fakeActor) Reencrypt(K kyber.Point, PK kyber.Point) (XhatEnc kyber.Point, err error) {
	return nil, f.reencryptErr
}

func (f fakeActor) VerifiableEncrypt(message []byte, GBar kyber.Point) (ciphertext types.Ciphertext, remainder []byte, err error) {
	return f.ct, nil, f.vencryptErr
}

func (f fakeActor) VerifiableDecrypt(ciphertexts []types.Ciphertext) ([][]byte, error) {
	return f.vdecryptData, f.vdecryptErr
}

func (f fakeActor) Reshare(co crypto.CollectiveAuthority, newThreshold int) error {
	return f.reshareErr
}

type fakeDKG struct {
	dkg.DKG

	err   error
	actor dkg.Actor
}

func (f fakeDKG) Listen() (dkg.Actor, error) {
	return f.actor, f.err
}

type badPoint struct {
	kyber.Point

	err  error
	data string
}

func (b badPoint) MarshalBinary() (data []byte, err error) {
	return []byte(b.data), b.err
}

type PointArray []kyber.Point
type badPointArray struct {
	PointArray

	err  error
	data string
}

func (b badPointArray) MarshalBinary() (data []byte, err error) {
	data = []byte(b.data)
	return data, b.err
}

type badScalar struct {
	kyber.Scalar

	err  error
	data string
}

func (b badScalar) MarshalBinary() (data []byte, err error) {
	return []byte(b.data), b.err
}

type fakeMino struct {
	mino.Mino

	addr mino.Address
}

func (f fakeMino) GetAddress() mino.Address {
	return f.addr
}
