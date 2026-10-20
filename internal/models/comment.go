package models

import "time"

type Comment struct {
	ID        int64
	NoteID    int64
	Content   string
	AuthorID  int64
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}
