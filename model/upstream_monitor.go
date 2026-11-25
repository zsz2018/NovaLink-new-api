package model

// Upstream monitor secrets and frozen execution inputs never appear in JSON.
// Business JSON uses TEXT so the same schema works on all primary databases.
type MonitorConfiguration struct {
	ID      int      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	Version int64    `json:"version"`
	Content LongText `json:"-"`
	Secrets LongText `json:"-"`
}

type MonitorDevice struct {
	ID          string `json:"id" gorm:"type:varchar(64);primaryKey"`
	OwnerID     int    `json:"owner_id" gorm:"index"`
	Name        string `json:"name" gorm:"type:varchar(128)"`
	PairHash    string `json:"-" gorm:"type:varchar(64);index"`
	PairExpires int64  `json:"-"`
	TokenHash   string `json:"-" gorm:"type:varchar(64);index"`
	ActiveJob   string `json:"-" gorm:"type:varchar(64)"`
	ExpiresAt   int64  `json:"expires_at"`
	CreatedAt   int64  `json:"created_at"`
	LastSeenAt  int64  `json:"last_seen_at"`
	RevokedAt   int64  `json:"revoked_at"`
}

type MonitorJob struct {
	ID            string   `json:"id" gorm:"type:varchar(64);primaryKey"`
	AccountID     string   `json:"account_id" gorm:"type:varchar(64);index"`
	DeviceID      string   `json:"device_id" gorm:"type:varchar(64);index"`
	FlowID        string   `json:"flow_id" gorm:"type:varchar(64)"`
	Mode          string   `json:"mode" gorm:"type:varchar(16)"`
	Status        string   `json:"status" gorm:"type:varchar(32);index"`
	Control       string   `json:"control" gorm:"type:varchar(32)"`
	Step          int      `json:"step"`
	Message       string   `json:"message" gorm:"type:text"`
	Complete      bool     `json:"complete"`
	ConfigVersion int64    `json:"config_version"`
	FlowRevision  int64    `json:"flow_revision"`
	Frozen        LongText `json:"-"`
	LeaseHash     string   `json:"-" gorm:"type:varchar(64)"`
	LeaseUntil    int64    `json:"lease_until"`
	RecordedSteps LongText `json:"-"`
	CreatedAt     int64    `json:"created_at" gorm:"index"`
	UpdatedAt     int64    `json:"updated_at"`
}

type MonitorSnapshot struct {
	ID          string   `json:"id" gorm:"type:varchar(64);primaryKey"`
	JobID       string   `json:"job_id" gorm:"type:varchar(64);index"`
	AccountID   string   `json:"account_id" gorm:"type:varchar(64);index"`
	Data        LongText `json:"-"`
	Confirmed   bool     `json:"confirmed" gorm:"index"`
	Complete    bool     `json:"complete"`
	CreatedAt   int64    `json:"created_at" gorm:"index"`
	ConfirmedAt int64    `json:"confirmed_at"`
}
