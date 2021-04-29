package types

import (
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
)

//Todo : redefine structs using non-raw types

var msgFormats = registry.NewSimpleRegistry()

// RegisterMessageFormat register the engine for the provided format.
func RegisterMessageFormat(c serde.Format, f serde.FormatEngine) {
	msgFormats.Register(c, f)
}

type ID string

//todo : status should be string ?
type status uint16
const(
	//Initial = 0
	Open = 1
	Closed = 2
	ShuffledBallots = 3
	//DecryptedBallots = 4
	ResultAvailable = 5
	Canceled = 6
)

// SimpleElection contains all information about a simple election
type SimpleElection struct {
	Title            string
	ElectionID       ID
	AdminId			 string
	Candidates       []string
	Status           status // Initial | Open | Closed | Shuffling | Decrypting | ..
	Pubkey           []byte
	EncryptedBallots map[string][]byte
	//todo : all shuffled ballots [][][]byte
	ShuffledBallots  [][]byte
	//todo : all proofs [][]byte
	Proof 			 []byte
	DecryptedBallots []SimpleBallot
}

// SimpleBallot contains all information about a simple ballot
type SimpleBallot struct {
	Vote string
}

/*
// Election contains all information about an election
//
// - implements serde.Message
type Election struct {
	ElectionId       ID
	AdminId			 ID
	Configuration    Configuration
	Status           status // Initial | Open | Closed | Shuffling | Decrypting | ..
	Pubkey           []byte
	EncryptedBallots map[ID][]byte
	ShuffledBallots  [][]byte
	DecryptedBallots []Ballot
}


// NewElection creates a new Election.
func NewElection(ElectionId ID, AdminId ID, Configuration Configuration, Status status, Pubkey []byte,
	EncryptedBallots map[ID][]byte, ShuffledBallots [][]byte, DecryptedBallots []Ballot) Election {
	return Election{
		ElectionId:       ElectionId,
		AdminId:          AdminId,
		Configuration:    Configuration,
		Status:           Status,
		Pubkey:           Pubkey,
		EncryptedBallots: EncryptedBallots,
		ShuffledBallots:  ShuffledBallots,
		DecryptedBallots: DecryptedBallots,
	}
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data for the Election message.
func (e Election) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, e)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode election: %v", err)
	}

	return data, nil
}

// Ballot contains the vote
//
// - implements serde.Message
type Ballot struct {

	// SelectResult contains the result of each Select question. The result of a
	// select is a list of boolean that says for each choice if it has been
	// selected or not.
	SelectResult map[ID][]bool

	// RankResult contains the result of each Rank question. The result of a
	// rank question is the list of ranks for each choice. A choice that hasn't
	// been ranked will have a value < 0.
	RankResult map[ID][]int

	// Text Result contains the result of each Text question. The result of a
	// text question is the list of text answer for each choice.
	TextResult map[ID][]string
}

// NewBallot creates a new Ballot.
func NewBallot(SelectResult map[ID][]bool, RankResult map[ID][]int, TextResult map[ID][]string) Ballot {
	return Ballot{
		SelectResult: SelectResult,
		RankResult:   RankResult,
		TextResult:   TextResult,
	}
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data for the Ballot message.
func (b Ballot) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, b)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode ballot: %v", err)
	}

	return data, nil
}

// Configuration contains the configuration of a new poll.
type Configuration struct {
	MainTitle string
	Scaffold  []Subject
}

// NewConfiguration creates a new Configuration.
func NewConfiguration(MainTitle string, Scaffold []Subject) Configuration {
	return Configuration{
		MainTitle: MainTitle,
		Scaffold:  Scaffold,
	}
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data for the Configuration message.
func (c Configuration) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, c)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode configuration: %v", err)
	}

	return data, nil
}


// Subject is a wrapper around multiple questions that can be of type "select",
// "rank", or "text".
type Subject struct {
	ID ID

	Title string

	// Order defines the order of the different question, which all have a uniq
	// identifier. This is purely for display purpose.
	Order []ID

	Subjects []Subject
	Selects  []Select
	Ranks    []Rank
	Texts    []Text
}

// NewSubject creates a new Subject.
func NewSubject(ID ID, Title string, Order []ID, Subjects []Subject, Selects []Select,
	Ranks []Rank, Texts []Text) Subject {
	return Subject{
		ID: ID,
		Title:  Title,
		Order: Order,
		Subjects:  Subjects,
		Selects: Selects,
		Ranks:  Ranks,
		Texts: Texts,
	}
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data for the Subject message.
func (s Subject) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, s)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode subject: %v", err)
	}

	return data, nil
}

// Select describes a "select" question, which requires the user to select one
// or multiple choices.
type Select struct {
	ID ID

	Title   string
	MaxN    int
	MinN    int
	Choices []string
}

// NewSelect creates a new Select.
func NewSelect(ID ID, Title string, MaxN int, MinN int, Choices []string) Select {
	return Select{
		ID: ID,
		Title:  Title,
		MaxN: MaxN,
		MinN:  MinN,
		Choices: Choices,
	}
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data for the Select message.
func (s Select) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, s)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode select: %v", err)
	}

	return data, nil
}

// Rank describes a "rank" question, which requires the user to rank choices.
type Rank struct {
	ID ID

	Title   string
	MaxN    int
	MinN    int
	Choices []string
}

// NewRank creates a new Rank.
func NewRank(ID ID, Title string, MaxN int, MinN int, Choices []string) Rank {
	return Rank{
		ID: ID,
		Title:  Title,
		MaxN: MaxN,
		MinN:  MinN,
		Choices: Choices,
	}
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data for the Rank message.
func (r Rank) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, r)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode rank: %v", err)
	}

	return data, nil
}

// Text describes a "text" question, which allows the user to enter free text.
type Text struct {
	ID ID

	Title      string
	MaxN       int
	MinN       int
	MaxLength  int
	Regex      string
	Choices    []string
}

// NewText creates a new Text.
func NewText(ID ID, Title string, MaxN int, MinN int, MaxLength  int, Regex string,
	Choices []string) Text {
	return Text{
		ID: ID,
		Title:  Title,
		MaxN: MaxN,
		MinN:  MinN,
		MaxLength: MaxLength,
		Regex:  Regex,
		Choices: Choices,
	}
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data for the Text message.
func (t Text) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, t)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode text: %v", err)
	}

	return data, nil
}
*/