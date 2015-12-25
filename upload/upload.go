package upload

import "time"

type Upload struct {
	ID      uint64
	Started time.Time
	Image   string
	GotSig  bool
	GotACI  bool
	GotMan  bool
}

func NewUpload(name string) *Upload {
	return &Upload{Started: time.Now(), Image: name}
}
