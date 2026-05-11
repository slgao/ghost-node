package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/vpnplatform/core/internal/auth"
	"github.com/vpnplatform/core/internal/metrics"
	"github.com/vpnplatform/core/internal/service"
)

type Router struct {
	engine  *gin.Engine
	jwtSvc  *auth.JWTService
	rdb     *redis.Client
	authH   *AuthHandler
	nodeH   *NodeHandler
	userH   *UserHandler
	subH    *SubscriptionHandler
	usageH  *UsageHandler
	adminH  *AdminHandler
}

func NewRouter(
	jwtSvc *auth.JWTService,
	authSvc *service.AuthService,
	nodeSvc *service.NodeService,
	userSvc *service.UserService,
	trafficSvc *service.TrafficService,
	adminH *AdminHandler,
	rdb *redis.Client,
) *Router {
	return &Router{
		engine:  gin.New(),
		jwtSvc:  jwtSvc,
		rdb:     rdb,
		authH:   NewAuthHandler(authSvc),
		nodeH:   NewNodeHandler(nodeSvc, trafficSvc),
		userH:   NewUserHandler(userSvc),
		subH:    NewSubscriptionHandler(nodeSvc),
		usageH:  NewUsageHandler(trafficSvc),
		adminH:  adminH,
	}
}

func (r *Router) Setup() *gin.Engine {
	r.engine.Use(gin.Recovery())
	r.engine.Use(requestLogger())
	r.engine.Use(corsMiddleware())
	r.engine.Use(metrics.GinMiddleware())
	r.engine.Use(rateLimitByIP(r.rdb, "global", 300, time.Minute))

	// health + metrics
	r.engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().UTC()})
	})
	r.engine.GET("/metrics", gin.WrapH(promhttp.Handler()))

	v1 := r.engine.Group("/api/v1")

	// ── Public auth routes (stricter limit: 20 req/min) ───────────────────────
	authGroup := v1.Group("/auth", rateLimitByIP(r.rdb, "auth", 20, time.Minute))
	{
		authGroup.POST("/register", r.authH.Register)
		authGroup.POST("/login",    r.authH.Login)
		authGroup.POST("/refresh",  r.authH.Refresh)
		authGroup.POST("/logout",   r.authH.Logout)
	}

	// ── Authenticated user routes ─────────────────────────────────────────────
	protected := v1.Group("", auth.Required(r.jwtSvc))
	{
		protected.GET("/auth/me", r.authH.Me)

		protected.GET("/profile",           r.userH.GetProfile)
		protected.PUT("/profile/password",  r.userH.ChangePassword)

		protected.GET("/devices",         r.userH.ListDevices)
		protected.POST("/devices",        r.userH.AddDevice)
		protected.DELETE("/devices/:id",  r.userH.RemoveDevice)

		protected.GET("/nodes",                        r.nodeH.ListNodes)
		protected.GET("/nodes/:id",                    r.nodeH.GetNode)
		protected.GET("/nodes/:id/connect",            r.nodeH.GetConnectionConfig)
		protected.GET("/nodes/:id/subscription",       r.subH.GetSubscription)

		protected.GET("/usage", r.usageH.GetMyUsage)
	}

	// ── Admin routes ──────────────────────────────────────────────────────────
	admin := v1.Group("/admin", auth.Required(r.jwtSvc), auth.AdminOnly())
	{
		admin.GET("/nodes",                      r.nodeH.ListAllNodes)
		admin.POST("/nodes",                     r.nodeH.CreateNode)
		admin.DELETE("/nodes/:id",               r.nodeH.DeleteNode)
		admin.POST("/nodes/:id/transports",      r.nodeH.AddTransportProfile)

		admin.GET("/users",                      r.adminH.ListUsers)
		admin.PUT("/users/:id/status",           r.adminH.SetUserActive)
		admin.PUT("/users/:id/quota",            r.adminH.UpdateUserQuota)
	}

	return r.engine
}

func requestLogger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return ""
	})
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
