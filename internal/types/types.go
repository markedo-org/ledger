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
	ID                   string
	OwnerID              string
	OwnerSlug            string
	Slug                 string
	Title                string
	CreatedAt            time.Time
	ArchiveDoneAfterDays *int // nil means use the process default
	PurgeDoneAfterDays   *int // nil means use the process default
}

type Series struct {
	ID       string
	LedgerID string
	Prefix   string
	NextN    int
}

const (
	RoleWrite    = "write"
	RoleAdmin    = "admin"
	RoleOperator = "operator"
)

type Token struct {
	ID         string
	Actor      string
	OwnerID    string
	OwnerSlug  string
	LedgerID   string // empty means all ledgers for the owner
	LedgerSlug string
	Role       string
	Email      string
	CreatedAt  time.Time
}

func (t Token) IsOperator() bool { return t.Role == RoleOperator }

type Task struct {
	ID              string
	LedgerID        string
	SeriesID        string
	N               int
	Handle          string
	Title           string
	Body            string
	Phase           Phase
	Size            Size
	Rank            int
	Version         int
	Pushed          int
	VerifiedAt      *time.Time
	Since           time.Time
	Ref             string
	GateSignoff     bool
	GateThird       bool
	GateDecision    bool
	GateTime        bool
	ClaimedBy       string
	ClaimedUntil    *time.Time
	ClaimSecretHash string // stored hash; never shown
	ClaimID         string // plaintext, only on claim/heartbeat responses
	Evidence        string
	ClosedAt        *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Notes           []Note
	Checks          []Check
	Tags            []string
	DependsOn       []string
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

type Session struct {
	ID          string
	Actor       string
	GitHubID    string
	GitHubLogin string
	OwnerSlug   string
	LedgerSlug  string
	Role        string
	ExpiresAt   time.Time
	CreatedAt   time.Time
}

func (s Session) IsOperator() bool {
	if s.Role == RoleOperator {
		return true
	}
	// GitHub allowlist sessions are host-wide and have no owner binding.
	return s.GitHubLogin != "" && s.OwnerSlug == ""
}

// Covers reports whether this HTML session may see owner/ledger.
// An empty OwnerSlug is an operator session (GitHub allowlist) and sees all.
// A ledger binding is ignored when ledger is empty (owner index).
func (s Session) Covers(owner, ledger string) bool {
	if s.OwnerSlug == "" {
		return true
	}
	if owner != "" && s.OwnerSlug != owner {
		return false
	}
	if s.LedgerSlug != "" && ledger != "" && s.LedgerSlug != ledger {
		return false
	}
	return true
}
