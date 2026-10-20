package models

import "time"

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

type User struct {
	ID          int64
	OIDCSubject string
	Email       string
	DisplayName string
	Role        Role
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
