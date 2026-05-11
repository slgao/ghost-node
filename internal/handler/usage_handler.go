package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/vpnplatform/core/internal/auth"
	"github.com/vpnplatform/core/internal/service"
)

type UsageHandler struct {
	trafficSvc *service.TrafficService
}

func NewUsageHandler(trafficSvc *service.TrafficService) *UsageHandler {
	return &UsageHandler{trafficSvc: trafficSvc}
}

// GetMyUsage returns the authenticated user's bandwidth usage and quota.
// GET /api/v1/usage
func (h *UsageHandler) GetMyUsage(c *gin.Context) {
	uid := auth.UserIDFromContext(c)

	summary, err := h.trafficSvc.GetSummary(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch usage"})
		return
	}

	usedPct := 0.0
	if summary.QuotaBytes > 0 {
		usedPct = float64(summary.UsedBytes) / float64(summary.QuotaBytes) * 100
	}

	c.JSON(http.StatusOK, gin.H{
		"quota": gin.H{
			"plan":            summary.Plan,
			"quota_bytes":     summary.QuotaBytes,
			"used_bytes":      summary.UsedBytes,
			"remaining_bytes": summary.QuotaBytes - summary.UsedBytes,
			"used_percent":    usedPct,
			"expires_at":      summary.ExpiresAt,
		},
		"period": gin.H{
			"days":      30,
			"bytes_in":  summary.PeriodBytesIn,
			"bytes_out": summary.PeriodBytesOut,
			"total":     summary.PeriodBytesIn + summary.PeriodBytesOut,
		},
		"daily": summary.Daily,
	})
}
