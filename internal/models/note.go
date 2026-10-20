package models

import "time"

type Note struct {
	ID        int64
	Title     string
	Content   string
	AuthorID  int64
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}
