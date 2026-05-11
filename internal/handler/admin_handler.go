package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vpnplatform/core/internal/models"
	"github.com/vpnplatform/core/internal/repository"
)

type AdminHandler struct {
	userRepo repository.UserRepository
	db       *gorm.DB
}

func NewAdminHandler(userRepo repository.UserRepository, db *gorm.DB) *AdminHandler {
	return &AdminHandler{userRepo: userRepo, db: db}
}

// ListUsers returns paginated users with their subscription info.
// GET /api/v1/admin/users?page=1&page_size=20
func (h *AdminHandler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	users, total, err := h.userRepo.List(c.Request.Context(), offset, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"users":     users,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// SetUserActive enables or disables a user account.
// PUT /api/v1/admin/users/:id/status
func (h *AdminHandler) SetUserActive(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var req struct {
		IsActive bool `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.userRepo.FindByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	user.IsActive = req.IsActive
	if err := h.userRepo.Update(c.Request.Context(), user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

// UpdateUserQuota updates a user's subscription plan and bandwidth quota.
// PUT /api/v1/admin/users/:id/quota
func (h *AdminHandler) UpdateUserQuota(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var req struct {
		Plan       models.PlanType `json:"plan"`
		QuotaBytes int64           `json:"quota_bytes"`
		ExpiresAt  *time.Time      `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{"updated_at": time.Now()}
	if req.Plan != "" {
		updates["plan"] = req.Plan
	}
	if req.QuotaBytes > 0 {
		updates["bandwidth_quota"] = req.QuotaBytes
	}
	if req.ExpiresAt != nil {
		updates["expires_at"] = req.ExpiresAt
	}

	if err := h.db.WithContext(c.Request.Context()).
		Model(&models.Subscription{}).
		Where("user_id = ?", id).
		Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update quota"})
		return
	}

	var sub models.Subscription
	h.db.WithContext(c.Request.Context()).Where("user_id = ?", id).First(&sub)
	c.JSON(http.StatusOK, gin.H{"subscription": sub})
}
