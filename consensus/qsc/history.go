package qsc

import (
	"bytes"
	fmt "fmt"
	"strings"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric/encoding"
)

// epoch is a part of an history. In other words, it represents a time step in
// the QSC algorithm.
//
// - implements encoding.Packable
type epoch struct {
	hash   []byte
	random int64
}

// Pack implements encoding.Packable. It returns the protobuf message for an
// epoch.
func (e epoch) Pack() (proto.Message, error) {
	pb := &Epoch{
		Random: e.random,
		Hash:   e.hash,
	}

	return pb, nil
}

// Equal returns true when the other epoch is the same as the current one.
func (e epoch) Equal(other epoch) bool {
	if e.random != other.random {
		return false
	}
	if !bytes.Equal(e.hash, other.hash) {
		return false
	}

	return true
}

// history is an ordered list of epochs, or a list of QSC time steps.
//
// - implements encoding.Packable
// - implements fmt.Stringer
type history []epoch

func (h history) getLast() (epoch, bool) {
	if len(h) == 0 {
		return epoch{}, false
	}

	return h[len(h)-1], true
}

// Equal returns true when both histories are equal, false otherwise.
func (h history) Equal(other history) bool {
	if len(h) != len(other) {
		return false
	}

	for i, e := range h {
		if !e.Equal(other[i]) {
			return false
		}
	}

	return true
}

// Pack implements encoding.Packable. It returns the protobuf message for an
// history.
func (h history) Pack() (proto.Message, error) {
	pb := &History{
		Epochs: make([]*Epoch, len(h)),
	}

	for i, epoch := range h {
		packed, err := epoch.Pack()
		if err != nil {
			return nil, err
		}

		pb.Epochs[i] = packed.(*Epoch)
	}

	return pb, nil
}

// String implements fmt.Stringer. It returns a string representation of the
// history.
func (h history) String() string {
	epochs := make([]string, len(h))
	for i, e := range h {
		if len(e.hash) >= 2 {
			epochs[i] = fmt.Sprintf("%x", e.hash)[:4]
		} else {
			epochs[i] = "nil"
		}
	}
	return fmt.Sprintf("History[%d]{%s}", len(h), strings.Join(epochs, ","))
}

type histories []history

// getBest returns the best history in the set. The best history is defined such
// that the random value of the latest epoch is the highest for every last epoch
// in the histories. It returns nil if not history is found.
func (hists histories) getBest() history {
	best := -1
	random := int64(0)
	for i, h := range hists {
		last, ok := h.getLast()
		if ok && (best == -1 || last.random > random) {
			random = last.random
			best = i
		}
	}

	if best == -1 {
		// It happens if the histories are all empty.
		return nil
	}

	return hists[best]
}

// contains returns true when the given history is found in the set, otherwise
// it returns false. Two histories are considered the same if their last epochs
// are equal.
func (hists histories) contains(h history) bool {
	last, ok := h.getLast()
	if !ok {
		return false
	}

	for _, history := range hists {
		other, ok := history.getLast()
		if ok && last.Equal(other) {
			return true
		}
	}

	return false
}

// isUniqueBest returns true if the given history is uniquely best as defined in
// the histories.getBest function, false otherwise.
func (hists histories) isUniqueBest(h history) bool {
	last, ok := h.getLast()
	if !ok {
		return false
	}

	for _, history := range hists {
		other, ok := history.getLast()
		if ok {
			isEqual := history.Equal(h)

			if !isEqual && other.random >= last.random {
				return false
			}
		}
	}

	return true
}

type historiesFactory interface {
	FromMessageSet(set map[int64]*Message) (histories, error)
}

type defaultHistoriesFactory struct{}

func (f defaultHistoriesFactory) FromMessageSet(set map[int64]*Message) (histories, error) {
	hists := make(histories, 0, len(set))
	for _, msg := range set {
		hist := &History{}
		err := ptypes.UnmarshalAny(msg.GetValue(), hist)
		if err != nil {
			return nil, encoding.NewAnyDecodingError(hist, err)
		}

		epochs := make([]epoch, len(hist.GetEpochs()))
		for j, e := range hist.GetEpochs() {
			epochs[j] = epoch{
				random: e.GetRandom(),
				hash:   e.GetHash(),
			}
		}

		hists = append(hists, history(epochs))
	}

	return hists, nil
}
