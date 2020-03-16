package qsc

import (
	"bytes"
	fmt "fmt"
	"strings"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
)

type epoch struct {
	hash   []byte
	random int64
}

func (e epoch) Pack() (proto.Message, error) {
	pb := &Epoch{
		Random: e.random,
		Hash:   e.hash,
	}

	return pb, nil
}

func (e epoch) Equal(other epoch) bool {
	if e.random != other.random {
		return false
	}
	if !bytes.Equal(e.hash, other.hash) {
		return false
	}

	return true
}

type history []epoch

func (h history) getLast() (epoch, bool) {
	if len(h) == 0 {
		return epoch{}, false
	}

	return h[len(h)-1], true
}

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

func fromMessageSet(ms map[int64]*Message) (histories, error) {
	hists := make(histories, len(ms))
	for i, msg := range ms {
		hist := &History{}
		err := ptypes.UnmarshalAny(msg.GetValue(), hist)
		if err != nil {
			return nil, err
		}

		epochs := make([]epoch, len(hist.GetEpochs()))
		for j, e := range hist.GetEpochs() {
			epochs[j] = epoch{
				random: e.GetRandom(),
				hash:   e.GetHash(),
			}
		}

		hists[i] = history(epochs)
	}

	return hists, nil
}

func (hists histories) getBest() history {
	best := -1
	random := int64(0)
	for i, h := range hists {
		last, ok := h.getLast()
		if ok && last.random > random {
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
