package models

import "time"

type Object struct {
	ID         int        `json:"id" db:"id"`
	LastSeenAt *time.Time `json:"last_seen_at" db:"last_seen_at"`
	Online     bool       `json:"online" db:"-"`
}

type ObjectsInput struct {
	ObjectIDs []int `json:"object_ids"`
}
