package api

import "notebook/internal/models"

func isOwnerOrAdmin(user *models.User, authorID int64) bool {
	if user == nil {
		return false
	}
	return user.ID == authorID || user.Role == models.RoleAdmin
}

func isAdmin(user *models.User) bool {
	return user != nil && user.Role == models.RoleAdmin
}
