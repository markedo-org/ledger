package types

import "time"

type Phase string

const (
	PhaseNOW    Phase = "NOW"
	PhaseNEXT   Phase = "NEXT"
	PhaseLATER  Phase = "LATER"
	PhaseGATED  Phase = "GATED"
	PhasePARKED Phase = "PARKED"
	PhaseDONE   Phase = "DONE"
)

func (p Phase) Rank() int {
	switch p {
	case PhaseNOW:
		return 0
	case PhaseNEXT:
		return 1
	case PhaseLATER:
		return 2
	case PhaseGATED:
		return 3
	case PhasePARKED:
		return 4
	case PhaseDONE:
		return 5
	default:
		return -1
	}
}

func ParsePhase(s string) (Phase, bool) {
	p := Phase(s)
	return p, p.Rank() >= 0
}

type Size string

const (
	SizeNone Size = ""
	SizeS    Size = "S"
	SizeM    Size = "M"
	SizeL    Size = "L"
)

func ParseSize(s string) (Size, bool) {
	switch s {
	case "", "S", "M", "L":
		return Size(s), true
	case "-":
		return SizeNone, true
	default:
		return "", false
	}
}

type Owner struct {
	ID         string
	Slug       string
	MaxLedgers int
	CreatedAt  time.Time
}

type Ledger struct {
	ID        string
	OwnerID   string
	OwnerSlug string
	Slug      string
	Title     string
	CreatedAt time.Time
}

type Series struct {
	ID       string
	LedgerID string
	Prefix   string
	NextN    int
}

type Token struct {
	ID        string
	Actor     string
	OwnerID   string
	LedgerID  string // empty means all ledgers for the owner
	Role      string
	CreatedAt time.Time
}

type Task struct {
	ID           string
	LedgerID     string
	SeriesID     string
	N            int
	Handle       string
	Title        string
	Body         string
	Phase        Phase
	Size         Size
	Rank         int
	Version      int
	Pushed       int
	VerifiedAt   *time.Time
	Since        time.Time
	Ref          string
	GateSignoff  bool
	GateThird    bool
	GateDecision bool
	GateTime     bool
	ClaimedBy    string
	ClaimedUntil *time.Time
	Evidence     string
	ClosedAt     *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Notes        []Note
	Checks       []Check
	DependsOn    []string
}

type Note struct {
	ID        string
	TaskID    string
	Actor     string
	Body      string
	CreatedAt time.Time
}

type Check struct {
	ID     string
	TaskID string
	Body   string
	Done   bool
	Rank   int
}

type Event struct {
	ID        int64
	LedgerID  string
	TaskID    string
	Actor     string
	Kind      string
	Payload   string
	CreatedAt time.Time
}
