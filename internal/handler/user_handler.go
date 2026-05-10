package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/vpnplatform/core/internal/auth"
	"github.com/vpnplatform/core/internal/models"
	"github.com/vpnplatform/core/internal/service"
)

type UserHandler struct {
	userSvc *service.UserService
}

func NewUserHandler(userSvc *service.UserService) *UserHandler {
	return &UserHandler{userSvc: userSvc}
}

// GetProfile returns the authenticated user's profile.
// GET /api/v1/profile
func (h *UserHandler) GetProfile(c *gin.Context) {
	uid := auth.UserIDFromContext(c)
	user, err := h.userSvc.GetProfile(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

// ChangePassword updates the authenticated user's password.
// PUT /api/v1/profile/password
func (h *UserHandler) ChangePassword(c *gin.Context) {
	uid := auth.UserIDFromContext(c)
	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.userSvc.ChangePassword(c.Request.Context(), uid, req.OldPassword, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "password updated"})
}

// ListDevices returns all devices for the authenticated user.
// GET /api/v1/devices
func (h *UserHandler) ListDevices(c *gin.Context) {
	uid := auth.UserIDFromContext(c)
	devices, err := h.userSvc.ListDevices(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list devices"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"devices": devices})
}

// AddDevice registers a new device for the authenticated user.
// POST /api/v1/devices
func (h *UserHandler) AddDevice(c *gin.Context) {
	uid := auth.UserIDFromContext(c)
	var req struct {
		Name      string            `json:"name"       binding:"required"`
		Type      models.DeviceType `json:"type"       binding:"required"`
		PublicKey string            `json:"public_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	device, err := h.userSvc.AddDevice(c.Request.Context(), uid, service.AddDeviceInput{
		Name:      req.Name,
		Type:      req.Type,
		PublicKey: req.PublicKey,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"device": device})
}

// RemoveDevice deactivates a device.
// DELETE /api/v1/devices/:id
func (h *UserHandler) RemoveDevice(c *gin.Context) {
	uid := auth.UserIDFromContext(c)

	deviceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device id"})
		return
	}

	if err := h.userSvc.RemoveDevice(c.Request.Context(), uid, deviceID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "device removed"})
}
