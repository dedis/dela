package qsc

import (
	"bytes"
	fmt "fmt"
	"strings"

	"go.dedis.ch/dela/consensus/qsc/json"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// epoch is a part of an history. In other words, it represents a time step in
// the QSC algorithm.
//
// - implements serde.Message
type epoch struct {
	serde.UnimplementedMessage

	hash   []byte
	random int64
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
// - implements serde.Message
// - implements fmt.Stringer
type history struct {
	serde.UnimplementedMessage

	epochs []epoch
}

func (h history) getLast() (epoch, bool) {
	if len(h.epochs) == 0 {
		return epoch{}, false
	}

	return h.epochs[len(h.epochs)-1], true
}

// Equal returns true when both histories are equal, false otherwise.
func (h history) Equal(other history) bool {
	if len(h.epochs) != len(other.epochs) {
		return false
	}

	for i, e := range h.epochs {
		if !e.Equal(other.epochs[i]) {
			return false
		}
	}

	return true
}

// VisitJSON implements serde.Message. It serializes the history in JSON format.
func (h history) VisitJSON(serde.Serializer) (interface{}, error) {
	epochs := make([]json.Epoch, len(h.epochs))
	for i, epoch := range h.epochs {
		epochs[i] = json.Epoch{
			Hash:   epoch.hash,
			Random: epoch.random,
		}
	}

	return json.History(epochs), nil
}

// String implements fmt.Stringer. It returns a string representation of the
// history.
func (h history) String() string {
	epochs := make([]string, len(h.epochs))
	for i, e := range h.epochs {
		if len(e.hash) >= 2 {
			epochs[i] = fmt.Sprintf("%x", e.hash)[:4]
		} else {
			epochs[i] = "nil"
		}
	}
	return fmt.Sprintf("History[%d]{%s}", len(h.epochs), strings.Join(epochs, ","))
}

// HistoryFactory is a message factory to decode histories.
//
// - implements serde.Factory
type HistoryFactory struct {
	serde.UnimplementedFactory
}

// VisitJSON implements serde.Factory. It deserializes the history in JSON
// format.
func (f HistoryFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.History{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	epochs := make([]epoch, len(m))
	for i, e := range m {
		epochs[i] = epoch{
			hash:   e.Hash,
			random: e.Random,
		}
	}

	return history{epochs: epochs}, nil
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
		return history{}
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
		// We skip to avoid comparing h to itself, which would make this
		// function always return false
		if history.Equal(h) {
			continue
		}

		other, ok := history.getLast()
		if ok && other.random >= last.random {
			return false
		}
	}

	return true
}

type historiesFactory interface {
	FromMessageSet(set map[int64]Message) (histories, error)
}

type defaultHistoriesFactory struct{}

func (f defaultHistoriesFactory) FromMessageSet(set map[int64]Message) (histories, error) {
	hists := make(histories, 0, len(set))
	for _, msg := range set {
		hist := msg.value.(history)

		epochs := make([]epoch, len(hist.epochs))
		for j, e := range hist.epochs {
			epochs[j] = epoch{
				random: e.random,
				hash:   e.hash,
			}
		}

		hists = append(hists, history{epochs: epochs})
	}

	return hists, nil
}
