package types

type CreateElectionTransaction struct {
	ElectionID string
	Title      string
	AdminId    string
	Candidates []string
	PublicKey  []byte
}

type CastVoteTransaction struct {
	ElectionID string
	UserId     string
	Ballot     []byte
}

type CloseElectionTransaction struct {
	ElectionID string
	UserId     string
}

type ShuffleBallotsTransaction struct {
	ElectionID      string
	Round           int
	ShuffledBallots [][]byte
	Proof           []byte
	Node            string
}

type DecryptBallotsTransaction struct {
	ElectionID       string
	UserId           string
	DecryptedBallots []Ballot
}

type CancelElectionTransaction struct {
	ElectionID string
	UserId     string
}
