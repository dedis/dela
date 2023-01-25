package pedersen

import (
	"crypto/sha256"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/kyber/v3"
	"golang.org/x/xerrors"
)

// A lot of this code deals with low-level crypto to verify DKG proofs.
// TODO: move low-level DKG proof verification functions to Kyber. (#241)

// checkDecryptionProof verifies the decryption proof.
//
// See https://arxiv.org/pdf/2205.08529.pdf / section 5.4 Protocol / step 3
func checkDecryptionProof(sp types.ShareAndProof, K kyber.Point) error {

	tmp1 := suite.Point().Mul(sp.Fi, K)
	tmp2 := suite.Point().Mul(sp.Ei, sp.Ui)
	UHat := suite.Point().Sub(tmp1, tmp2)

	tmp1 = suite.Point().Mul(sp.Fi, nil)
	tmp2 = suite.Point().Mul(sp.Ei, sp.Hi)
	HHat := suite.Point().Sub(tmp1, tmp2)

	hash := sha256.New()
	sp.Ui.MarshalTo(hash)
	UHat.MarshalTo(hash)
	HHat.MarshalTo(hash)
	tmp := suite.Scalar().SetBytes(hash.Sum(nil))

	if !tmp.Equal(sp.Ei) {
		return xerrors.Errorf("hash is not valid: %x != %x", sp.Ei, tmp)
	}

	return nil
}

// checkEncryptionProof verifies the encryption proofs.
//
// See https://arxiv.org/pdf/2205.08529.pdf / section 5.4 Protocol / step 3
func checkEncryptionProof(cp types.Ciphertext) error {

	tmp1 := suite.Point().Mul(cp.F, nil)
	tmp2 := suite.Point().Mul(cp.E, cp.K)
	w := suite.Point().Sub(tmp1, tmp2)

	tmp1 = suite.Point().Mul(cp.F, cp.GBar)
	tmp2 = suite.Point().Mul(cp.E, cp.UBar)
	wBar := suite.Point().Sub(tmp1, tmp2)

	hash := sha256.New()
	cp.C.MarshalTo(hash)
	cp.K.MarshalTo(hash)
	cp.UBar.MarshalTo(hash)
	w.MarshalTo(hash)
	wBar.MarshalTo(hash)

	tmp := suite.Scalar().SetBytes(hash.Sum(nil))
	if !tmp.Equal(cp.E) {
		return xerrors.Errorf("hash not valid: %x != %x", cp.E, tmp)
	}

	return nil
}

// verifiableDecryption generates the decryption shares as well as the
// decryption proof.
//
// See https://arxiv.org/pdf/2205.08529.pdf / section 5.4 Protocol / step 3
func verifiableDecryption(ct types.Ciphertext, V kyber.Scalar, I int) (*types.ShareAndProof, error) {
	err := checkEncryptionProof(ct)
	if err != nil {
		return nil, xerrors.Errorf("failed to check proof: %v", err)
	}

	ui := suite.Point().Mul(V, ct.K)

	// share of this party, needed for decrypting
	partial := suite.Point().Sub(ct.C, ui)

	si := suite.Scalar().Pick(suite.RandomStream())
	UHat := suite.Point().Mul(si, ct.K)
	HHat := suite.Point().Mul(si, nil)

	hash := sha256.New()
	ui.MarshalTo(hash)
	UHat.MarshalTo(hash)
	HHat.MarshalTo(hash)
	Ei := suite.Scalar().SetBytes(hash.Sum(nil))

	Fi := suite.Scalar().Add(si, suite.Scalar().Mul(Ei, V))
	Hi := suite.Point().Mul(V, nil)

	sp := types.ShareAndProof{
		V:  partial,
		I:  int64(I),
		Ui: ui,
		Ei: Ei,
		Fi: Fi,
		Hi: Hi,
	}

	return &sp, nil
}
