package lottery

import "time"

// SGSSCBackfillItem is a durable, bounded work queue, not an alternate draw
// source. Attempts is also the fencing generation for late worker responses.
type SGSSCBackfillItem struct {
	Issue          string     `gorm:"primaryKey;size:11" json:"issue"`
	DrawAt         time.Time  `json:"draw_at"`
	Status         string     `json:"status"`
	Reason         string     `json:"reason"`
	Attempts       int        `json:"attempts"`
	LastError      string     `json:"last_error"`
	NextRetryAt    time.Time  `json:"next_retry_at"`
	LeaseUntil     *time.Time `json:"-"`
	RequestedBy    string     `json:"-"`
	RequestTrigger string     `json:"-"`
	RequestID      string     `json:"-"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (SGSSCBackfillItem) TableName() string { return "lottery_sgssc_backfill_items" }

// Every claim has its own journal entry. Completing an attempt never replaces
// the outcome of an earlier attempt, including failures and abandoned leases.
type SGSSCBackfillAttempt struct {
	ID                 uint64     `gorm:"primaryKey" json:"id"`
	Issue              string     `json:"issue"`
	Attempt            int        `json:"attempt"`
	Status             string     `json:"status"`
	Trigger            string     `json:"trigger"`
	Operator           string     `json:"operator"`
	RequestID          string     `json:"request_id"`
	StartedAt          time.Time  `json:"started_at"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
	Numbers            string     `json:"numbers"`
	Imported           bool       `json:"imported"`
	SettledBets        int64      `json:"settled_bets"`
	Error              string     `json:"error"`
	SourceRevision     string     `json:"source_revision"`
	ConversionRevision string     `json:"conversion_revision"`
}

func (SGSSCBackfillAttempt) TableName() string { return "lottery_sgssc_backfill_attempts" }
