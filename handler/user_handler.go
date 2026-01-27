package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

type userCreateRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6"`
	Role     string `json:"role"`
	Status   string `json:"status"`
}

type userUpdateRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Role     string `json:"role" binding:"required"`
	Status   string `json:"status" binding:"required"`
}

type userPasswordRequest struct {
	Password string `json:"password" binding:"required,min=6"`
}

func (h *Handler) ListUsers(c *gin.Context) {
	_, role := getUserContext(c)
	if !isAdminRole(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	users, err := database.ListUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list users: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"users": users})
}

func (h *Handler) CreateUser(c *gin.Context) {
	_, role := getUserContext(c)
	if !isAdminRole(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	var req userCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	roleValue := strings.ToLower(strings.TrimSpace(req.Role))
	if roleValue == "" {
		roleValue = "user"
	}
	statusValue := strings.ToLower(strings.TrimSpace(req.Status))
	if statusValue == "" {
		statusValue = "active"
	}
	if roleValue != "admin" && roleValue != "user" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role"})
		return
	}
	if statusValue != "active" && statusValue != "disabled" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status"})
		return
	}

	if err := database.CreateUserWithRoleStatus(req.Username, req.Password, roleValue, statusValue); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user: " + err.Error()})
		return
	}

	user, err := database.GetUser(req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load user"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"user": user})
}

func (h *Handler) UpdateUser(c *gin.Context) {
	_, role := getUserContext(c)
	if !isAdminRole(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	id, err := parseIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user id"})
		return
	}

	var req userUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	roleValue := strings.ToLower(strings.TrimSpace(req.Role))
	statusValue := strings.ToLower(strings.TrimSpace(req.Status))
	if roleValue != "admin" && roleValue != "user" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role"})
		return
	}
	if statusValue != "active" && statusValue != "disabled" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status"})
		return
	}

	user := &database.User{
		ID:       id,
		Username: req.Username,
		Role:     roleValue,
		Status:   statusValue,
	}

	if err := database.UpdateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user: " + err.Error()})
		return
	}

	updated, err := database.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": updated})
}

func (h *Handler) UpdateUserPassword(c *gin.Context) {
	_, role := getUserContext(c)
	if !isAdminRole(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	id, err := parseIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user id"})
		return
	}

	var req userPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	if err := database.UpdateUserPassword(id, req.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password updated"})
}

func (h *Handler) UpdateUserStatus(c *gin.Context) {
	_, role := getUserContext(c)
	if !isAdminRole(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	id, err := parseIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user id"})
		return
	}

	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	statusValue := strings.ToLower(strings.TrimSpace(req.Status))
	if statusValue != "active" && statusValue != "disabled" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status"})
		return
	}

	if err := database.UpdateUserStatus(id, statusValue); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update status: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Status updated"})
}

func (h *Handler) DeleteUser(c *gin.Context) {
	userID, role := getUserContext(c)
	if !isAdminRole(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	id, err := parseIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user id"})
		return
	}

	if id == userID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete current user"})
		return
	}

	toDelete, err := database.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	if strings.ToLower(strings.TrimSpace(toDelete.Role)) == "admin" {
		adminCount, err := database.CountAdmins()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check admin count"})
			return
		}
		if adminCount <= 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete last admin"})
			return
		}
	}

	if err := database.DeleteUser(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deleted"})
}

func parseIDParam(c *gin.Context, name string) (int64, error) {
	value := c.Param(name)
	return strconv.ParseInt(value, 10, 64)
}
