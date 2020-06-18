package types

import (
	"bytes"
	"fmt"
	"strings"

	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serdeng"
	"go.dedis.ch/dela/serdeng/registry"
)

var historyFormats = registry.NewSimpleRegistry()

func RegisterHistoryFormat(c serdeng.Codec, f serdeng.Format) {
	historyFormats.Register(c, f)
}

// Epoch is a part of an history. In other words, it represents a time step in
// the QSC algorithm.
type Epoch struct {
	hash   []byte
	random int64
}

func NewEpoch(hash []byte, random int64) Epoch {
	return Epoch{
		hash:   hash,
		random: random,
	}
}

func (e Epoch) GetHash() []byte {
	return append([]byte{}, e.hash...)
}

func (e Epoch) GetRandom() int64 {
	return e.random
}

// Equal returns true when the other epoch is the same as the current one.
func (e Epoch) Equal(other Epoch) bool {
	if e.random != other.random {
		return false
	}
	if !bytes.Equal(e.hash, other.hash) {
		return false
	}

	return true
}

// History is an ordered list of epochs, or a list of QSC time steps.
//
// - implements serde.Message
// - implements fmt.Stringer
type History struct {
	epochs []Epoch
}

func NewHistory(epochs ...Epoch) History {
	return History{
		epochs: epochs,
	}
}

func (h History) GetEpochs() []Epoch {
	return append([]Epoch{}, h.epochs...)
}

func (h History) GetLast() (Epoch, bool) {
	if len(h.epochs) == 0 {
		return Epoch{}, false
	}

	return h.epochs[len(h.epochs)-1], true
}

// Equal returns true when both histories are equal, false otherwise.
func (h History) Equal(other History) bool {
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

// Serialize implements serde.Message.
func (h History) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := historyFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, h)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// String implements fmt.Stringer. It returns a string representation of the
// history.
func (h History) String() string {
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

// Deserialize implements serde.Factory.
func (f HistoryFactory) Deserialize(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
	format := historyFormats.Get(ctx.GetName())

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

type Histories []History

func NewHistories(set map[int64]Message) Histories {
	hists := make(Histories, 0, len(set))
	for _, msg := range set {
		hist := msg.value.(History)

		epochs := make([]Epoch, len(hist.epochs))
		for j, e := range hist.epochs {
			epochs[j] = Epoch{
				random: e.random,
				hash:   e.hash,
			}
		}

		hists = append(hists, History{epochs: epochs})
	}

	return hists
}

// GetBest returns the best history in the set. The best history is defined such
// that the random value of the latest epoch is the highest for every last epoch
// in the histories. It returns nil if not history is found.
func (hists Histories) GetBest() History {
	best := -1
	random := int64(0)
	for i, h := range hists {
		last, ok := h.GetLast()
		if ok && (best == -1 || last.random > random) {
			random = last.random
			best = i
		}
	}

	if best == -1 {
		// It happens if the histories are all empty.
		return History{}
	}

	return hists[best]
}

// Contains returns true when the given history is found in the set, otherwise
// it returns false. Two histories are considered the same if their last epochs
// are equal.
func (hists Histories) Contains(h History) bool {
	last, ok := h.GetLast()
	if !ok {
		return false
	}

	for _, history := range hists {
		other, ok := history.GetLast()
		if ok && last.Equal(other) {
			return true
		}
	}

	return false
}

// IsUniqueBest returns true if the given history is uniquely best as defined in
// the histories.getBest function, false otherwise.
func (hists Histories) IsUniqueBest(h History) bool {
	last, ok := h.GetLast()
	if !ok {
		return false
	}

	for _, history := range hists {
		// We skip to avoid comparing h to itself, which would make this
		// function always return false
		if history.Equal(h) {
			continue
		}

		other, ok := history.GetLast()
		if ok && other.random >= last.random {
			return false
		}
	}

	return true
}
