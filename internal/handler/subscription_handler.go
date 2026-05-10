package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/vpnplatform/core/internal/models"
	"github.com/vpnplatform/core/internal/service"
)

// SubscriptionHandler generates client connection configs in multiple formats.
type SubscriptionHandler struct {
	nodeSvc *service.NodeService
}

func NewSubscriptionHandler(nodeSvc *service.NodeService) *SubscriptionHandler {
	return &SubscriptionHandler{nodeSvc: nodeSvc}
}

// GetSubscription returns all transport profiles for a node as importable client configs.
// GET /api/v1/nodes/:id/subscription?format=vless|clash|singbox|all
func (h *SubscriptionHandler) GetSubscription(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node id"})
		return
	}

	node, err := h.nodeSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	format := c.DefaultQuery("format", "all")

	var uris []string
	for _, profile := range node.TransportProfiles {
		if !profile.IsActive {
			continue
		}
		uri := buildClientURI(node, &profile)
		if uri != "" {
			uris = append(uris, uri)
		}
	}

	switch format {
	case "vless":
		// Plain text list of VLESS URIs (one per line), base64-encoded for v2rayN/NekoRay
		raw := ""
		for _, u := range uris {
			raw += u + "\n"
		}
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.String(http.StatusOK, base64.StdEncoding.EncodeToString([]byte(raw)))

	case "clash":
		c.Header("Content-Type", "text/yaml; charset=utf-8")
		c.String(http.StatusOK, buildClashConfig(node, uris))

	case "singbox":
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, buildSingBoxConfig(node, uris))

	default: // "all"
		c.JSON(http.StatusOK, gin.H{
			"node":        node.Name,
			"region":      node.Region,
			"uris":        uris,
			"clash_url":   fmt.Sprintf("/api/v1/nodes/%s/subscription?format=clash",   id),
			"singbox_url": fmt.Sprintf("/api/v1/nodes/%s/subscription?format=singbox", id),
			"v2rayn_url":  fmt.Sprintf("/api/v1/nodes/%s/subscription?format=vless",   id),
		})
	}
}

// buildClientURI constructs a protocol URI from a transport profile's config JSON.
func buildClientURI(node *models.Node, profile *models.TransportProfile) string {
	cfg := map[string]interface{}(profile.Config)
	if cfg == nil {
		return ""
	}

	getString := func(key string) string {
		if v, ok := cfg[key]; ok {
			return fmt.Sprintf("%v", v)
		}
		return ""
	}

	switch profile.Type {
	case models.TransportXray:
		proto := getString("protocol")
		transport := getString("transport")

		switch {
		case proto == "vless" && transport == "reality":
			return buildVLESSRealityURI(node, profile, cfg, getString)
		case proto == "vless" && (transport == "ws" || transport == "websocket"):
			return buildVLESSWSURI(node, profile, cfg, getString)
		case proto == "vless" && transport == "grpc":
			return buildVLESSGRPCURI(node, profile, cfg, getString)
		}

	case models.TransportHysteria2:
		return buildHysteria2URI(node, profile, cfg, getString)
	}

	return ""
}

func buildVLESSRealityURI(node *models.Node, profile *models.TransportProfile, cfg map[string]interface{}, get func(string) string) string {
	uid := get("uuid")
	if uid == "" {
		return ""
	}
	params := url.Values{}
	params.Set("encryption", "none")
	params.Set("flow",       get("flow"))
	params.Set("security",   "reality")
	params.Set("sni",        get("server_name"))
	params.Set("fp",         "chrome")
	params.Set("pbk",        get("public_key"))
	params.Set("sid",        get("short_id"))
	params.Set("type",       "tcp")
	params.Set("headerType", "none")

	name := url.QueryEscape(fmt.Sprintf("%s-%s-REALITY", node.Name, node.Region))
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s", uid, node.Address, profile.Port, params.Encode(), name)
}

func buildVLESSWSURI(node *models.Node, profile *models.TransportProfile, cfg map[string]interface{}, get func(string) string) string {
	uid := get("uuid")
	if uid == "" {
		return ""
	}
	params := url.Values{}
	params.Set("encryption", "none")
	params.Set("security",   "tls")
	params.Set("type",       "ws")
	params.Set("host",       get("host"))
	params.Set("path",       get("path"))
	if sni := get("sni"); sni != "" {
		params.Set("sni", sni)
	}
	params.Set("fp", "chrome")

	name := url.QueryEscape(fmt.Sprintf("%s-%s-WS", node.Name, node.Region))
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s", uid, node.Address, profile.Port, params.Encode(), name)
}

func buildVLESSGRPCURI(node *models.Node, profile *models.TransportProfile, cfg map[string]interface{}, get func(string) string) string {
	uid := get("uuid")
	if uid == "" {
		return ""
	}
	params := url.Values{}
	params.Set("encryption",   "none")
	params.Set("security",     "tls")
	params.Set("type",         "grpc")
	params.Set("serviceName",  get("service_name"))
	params.Set("mode",         "gun")
	params.Set("fp",           "chrome")

	name := url.QueryEscape(fmt.Sprintf("%s-%s-gRPC", node.Name, node.Region))
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s", uid, node.Address, profile.Port, params.Encode(), name)
}

func buildHysteria2URI(node *models.Node, profile *models.TransportProfile, cfg map[string]interface{}, get func(string) string) string {
	password := get("password")
	if password == "" {
		return ""
	}
	params := url.Values{}
	if sni := get("sni"); sni != "" {
		params.Set("sni", sni)
	}
	if insecure := get("insecure"); insecure == "true" {
		params.Set("insecure", "1")
	}

	name := url.QueryEscape(fmt.Sprintf("%s-%s-HY2", node.Name, node.Region))
	return fmt.Sprintf("hysteria2://%s@%s:%d?%s#%s", password, node.Address, profile.Port, params.Encode(), name)
}

// buildClashConfig returns a minimal Clash Meta compatible YAML proxy group.
func buildClashConfig(node *models.Node, uris []string) string {
	proxies := ""
	names := ""
	for i, u := range uris {
		name := fmt.Sprintf("%s-%d", node.Name, i+1)
		proxies += fmt.Sprintf("  - {name: \"%s\", type: vless, server: \"%s\", uri: \"%s\"}\n", name, node.Address, u)
		names += fmt.Sprintf("      - %s\n", name)
	}

	return fmt.Sprintf(`# Clash Meta / Clash Verge — generated by VPN Platform
# Import via: Settings → Profiles → Import URL
mixed-port: 7890
allow-lan: false
mode: Rule
log-level: warning

proxies:
%s
proxy-groups:
  - name: "VPN"
    type: select
    proxies:
%s
rules:
  - GEOIP,CN,DIRECT
  - MATCH,VPN
`, proxies, names)
}

// buildSingBoxConfig returns a sing-box compatible JSON outbound.
func buildSingBoxConfig(node *models.Node, uris []string) string {
	outbounds := []map[string]interface{}{}
	for i, profile := range node.TransportProfiles {
		if !profile.IsActive {
			continue
		}
		cfg := map[string]interface{}(profile.Config)
		if cfg == nil {
			continue
		}
		getString := func(key string) string {
			if v, ok := cfg[key]; ok {
				return fmt.Sprintf("%v", v)
			}
			return ""
		}

		ob := map[string]interface{}{
			"tag":    fmt.Sprintf("%s-%d", node.Name, i+1),
			"type":   "vless",
			"server": node.Address,
			"port":   profile.Port,
			"uuid":   getString("uuid"),
			"flow":   getString("flow"),
		}

		if profile.Type == models.TransportXray && getString("transport") == "reality" {
			ob["tls"] = map[string]interface{}{
				"enabled":     true,
				"server_name": getString("server_name"),
				"utls": map[string]interface{}{
					"enabled":     true,
					"fingerprint": "chrome",
				},
				"reality": map[string]interface{}{
					"enabled":    true,
					"public_key": getString("public_key"),
					"short_id":   getString("short_id"),
				},
			}
		}
		outbounds = append(outbounds, ob)
	}

	_ = uris
	cfg := map[string]interface{}{
		"log": map[string]interface{}{"level": "warn"},
		"outbounds": append(outbounds, map[string]interface{}{
			"tag":  "direct",
			"type": "direct",
		}),
		"route": map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{"geoip": "cn", "outbound": "direct"},
			},
		},
	}

	b, _ := json.MarshalIndent(cfg, "", "  ")
	return string(b)
}
