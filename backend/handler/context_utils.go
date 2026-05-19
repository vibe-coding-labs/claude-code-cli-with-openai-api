package handler

import "github.com/gin-gonic/gin"

func getUserContext(c *gin.Context) (int64, string) {
	userIDValue, _ := c.Get("user_id")
	roleValue, _ := c.Get("role")

	var userID int64
	switch v := userIDValue.(type) {
	case int64:
		userID = v
	case int:
		userID = int64(v)
	case float64:
		userID = int64(v)
	}

	routeRole, ok := roleValue.(string)
	if !ok || routeRole == "" {
		routeRole = "user"
	}

	return userID, routeRole
}

func isAdminRole(role string) bool {
	return role == "admin"
}
