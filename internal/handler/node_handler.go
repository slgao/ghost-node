package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/vpnplatform/core/internal/models"
	"github.com/vpnplatform/core/internal/service"
)

type NodeHandler struct {
	nodeSvc *service.NodeService
}

func NewNodeHandler(nodeSvc *service.NodeService) *NodeHandler {
	return &NodeHandler{nodeSvc: nodeSvc}
}

// ListNodes returns all public online nodes.
// GET /api/v1/nodes
func (h *NodeHandler) ListNodes(c *gin.Context) {
	nodes, err := h.nodeSvc.ListOnline(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list nodes"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"nodes": nodes})
}

// GetNode returns a single node by ID.
// GET /api/v1/nodes/:id
func (h *NodeHandler) GetNode(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node id"})
		return
	}

	node, err := h.nodeSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"node": node})
}

// GetConnectionConfig returns the best transport profile for a node.
// GET /api/v1/nodes/:id/connect
func (h *NodeHandler) GetConnectionConfig(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node id"})
		return
	}

	profile, err := h.nodeSvc.GetConnectionConfig(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"profile": profile})
}

// ─── Admin endpoints ──────────────────────────────────────────────────────────

// CreateNode registers a new VPN node (admin only).
// POST /api/v1/admin/nodes
func (h *NodeHandler) CreateNode(c *gin.Context) {
	var req struct {
		Name    string `json:"name"    binding:"required"`
		Address string `json:"address" binding:"required"`
		Region  string `json:"region"  binding:"required"`
		Country string `json:"country"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	node, err := h.nodeSvc.Create(c.Request.Context(), service.CreateNodeInput{
		Name:    req.Name,
		Address: req.Address,
		Region:  req.Region,
		Country: req.Country,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"node": node})
}

// ListAllNodes returns all nodes regardless of status (admin only).
// GET /api/v1/admin/nodes
func (h *NodeHandler) ListAllNodes(c *gin.Context) {
	nodes, err := h.nodeSvc.ListAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list nodes"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"nodes": nodes})
}

// DeleteNode removes a node (admin only).
// DELETE /api/v1/admin/nodes/:id
func (h *NodeHandler) DeleteNode(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node id"})
		return
	}

	if err := h.nodeSvc.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "node deleted"})
}

// AddTransportProfile attaches a transport config to a node (admin only).
// POST /api/v1/admin/nodes/:id/transports
func (h *NodeHandler) AddTransportProfile(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node id"})
		return
	}

	var req struct {
		Type     models.TransportType   `json:"type"     binding:"required"`
		Port     int                    `json:"port"     binding:"required"`
		Config   map[string]interface{} `json:"config"`
		Priority int                    `json:"priority"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tp := &models.TransportProfile{
		Type:     req.Type,
		Port:     req.Port,
		Config:   models.JSONB(req.Config),
		Priority: req.Priority,
		IsActive: true,
	}

	if err := h.nodeSvc.AddTransportProfile(c.Request.Context(), id, tp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"profile": tp})
}
