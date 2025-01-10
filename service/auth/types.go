package auth

import "time"

type ApprovedEmail struct {
	Email   string
	Used    bool
	Created time.Time
	UsedAt  time.Time
}
