package auth

import "notebook/internal/models"

func ExtractRole(claims map[string]any, adminClaim, adminValue string) models.Role {
	val, ok := claims[adminClaim]
	if !ok {
		return models.RoleUser
	}

	switch v := val.(type) {
	case string:
		if v == adminValue {
			return models.RoleAdmin
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == adminValue {
				return models.RoleAdmin
			}
		}
	}
	return models.RoleUser
}
