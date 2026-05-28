package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/polarisagi/polarisagi-hermes/internal/repository/sqlite"
	"github.com/polarisagi/polarisagi-hermes/internal/service/channel"
	"github.com/polarisagi/polarisagi-hermes/internal/service/client"
	"github.com/polarisagi/polarisagi-hermes/internal/service/router"
	"github.com/polarisagi/polarisagi-hermes/internal/domain"
	modelsync "github.com/polarisagi/polarisagi-hermes/internal/service/sync"

	"github.com/polarisagi/polarisagi-hermes/pkg/logger"
)

var (
	Version         = "v2.0.0"
	oauthStateStore = sync.Map{}
)

// AdminHandler 处理后台管理面板的 RESTful API 请求
type AdminHandler struct {
	providerRepo *sqlite.ProviderRepo
	modelRepo    *sqlite.ModelRepo
	routeRepo    *sqlite.RouteRepo
	intentRepo   *sqlite.IntentRepo
	settingsRepo *sqlite.SettingsRepo
	clientSvc    *client.Manager

	// 热重载目标：写操作完成后同步刷新内存状态
	chanManager *channel.Manager
	pipeline    *router.Pipeline
}

func NewAdminHandler(
	pRepo *sqlite.ProviderRepo,
	mRepo *sqlite.ModelRepo,
	rRepo *sqlite.RouteRepo,
	iRepo *sqlite.IntentRepo,
	sRepo *sqlite.SettingsRepo,
	clientSvc *client.Manager,
	chanManager *channel.Manager,
	pipeline *router.Pipeline,
) *AdminHandler {
	return &AdminHandler{
		providerRepo: pRepo,
		modelRepo:    mRepo,
		routeRepo:    rRepo,
		intentRepo:   iRepo,
		settingsRepo: sRepo,
		clientSvc:    clientSvc,
		chanManager:  chanManager,
		pipeline:     pipeline,
	}
}

// reloadAll 在任何会影响路由或渠道的写操作后触发热重载
func (h *AdminHandler) reloadAll(ctx context.Context) {
	if err := h.chanManager.Reload(ctx); err != nil {
		slog.Warn("⚠️ [Admin] 渠道热重载失败", "error", err)
	}
	if err := h.pipeline.Reload(ctx); err != nil {
		slog.Warn("⚠️ [Admin] 路由管线热重载失败", "error", err)
	}
}

// reloadPipeline 仅刷新路由管线缓存（路由规则变更时使用）
func (h *AdminHandler) reloadPipeline(ctx context.Context) {
	if err := h.pipeline.Reload(ctx); err != nil {
		slog.Warn("⚠️ [Admin] 路由管线热重载失败", "error", err)
	}
}

// ---------------------------------------------------------
// 基础信息与存根 (Stubs) API
// ---------------------------------------------------------

func (h *AdminHandler) GetInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"version": Version,
		"debug":   false,
	})
}

func (h *AdminHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`[]`))
}

func (h *AdminHandler) GetClientsStatus(w http.ResponseWriter, r *http.Request) {
	statuses, err := h.clientSvc.GetAllStatuses(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"clients": statuses})
}

func (h *AdminHandler) ApplyClientConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Client string `json:"client"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Client == "" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := h.clientSvc.ApplyConfig(r.Context(), payload.Client); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"success"}`))
}

func (h *AdminHandler) RestoreClientConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Client string `json:"client"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Client == "" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := h.clientSvc.RestoreConfig(r.Context(), payload.Client); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"success"}`))
}

func (h *AdminHandler) HandleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		
		getIntSetting := func(key string, def int) int {
			valStr, _ := h.settingsRepo.GetSetting(r.Context(), key)
			if valStr == "" { return def }
			val, err := strconv.Atoi(valStr)
			if err != nil { return def }
			return val
		}

		settings := map[string]interface{}{
			"listen_addr": "127.0.0.1:27777",
			"breaker": map[string]int{
				"initial_cooldown_seconds": getIntSetting("initial_cooldown_seconds", 60),
				"max_cooldown_seconds":     getIntSetting("max_cooldown_seconds", 3600),
				"failure_threshold":        getIntSetting("failure_threshold", 3),
				"failure_window_seconds":   getIntSetting("failure_window_seconds", 120),
			},
			"google_oauth_client_id":     "",
			"google_oauth_client_secret": "",
		}
		
		val, _ := h.settingsRepo.GetSetting(r.Context(), "listen_addr")
		if val != "" { settings["listen_addr"] = val }
		val, _ = h.settingsRepo.GetSetting(r.Context(), "google_oauth_client_id")
		if val != "" { settings["google_oauth_client_id"] = val }
		val, _ = h.settingsRepo.GetSetting(r.Context(), "google_oauth_client_secret")
		if val != "" { settings["google_oauth_client_secret"] = val }
		
		_ = json.NewEncoder(w).Encode(settings)
	} else if r.Method == http.MethodPost {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
			if v, ok := payload["listen_addr"].(string); ok {
				_ = h.settingsRepo.SetSetting(r.Context(), "listen_addr", v)
			}
			if v, ok := payload["google_oauth_client_id"].(string); ok {
				_ = h.settingsRepo.SetSetting(r.Context(), "google_oauth_client_id", v)
			}
			if v, ok := payload["google_oauth_client_secret"].(string); ok {
				_ = h.settingsRepo.SetSetting(r.Context(), "google_oauth_client_secret", v)
			}
			if v, ok := payload["initial_cooldown_seconds"].(float64); ok {
				_ = h.settingsRepo.SetSetting(r.Context(), "initial_cooldown_seconds", strconv.Itoa(int(v)))
			}
			if v, ok := payload["max_cooldown_seconds"].(float64); ok {
				_ = h.settingsRepo.SetSetting(r.Context(), "max_cooldown_seconds", strconv.Itoa(int(v)))
			}
			if v, ok := payload["failure_threshold"].(float64); ok {
				_ = h.settingsRepo.SetSetting(r.Context(), "failure_threshold", strconv.Itoa(int(v)))
			}
			if v, ok := payload["failure_window_seconds"].(float64); ok {
				_ = h.settingsRepo.SetSetting(r.Context(), "failure_window_seconds", strconv.Itoa(int(v)))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}
}

func (h *AdminHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	logPath := logger.GetLogPath()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	data, err := os.ReadFile(logPath)
	if err != nil {
		_, _ = w.Write([]byte("No logs found or unable to read log file."))
		return
	}
	_, _ = w.Write(data)
}

func (h *AdminHandler) SetDebug(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"debug": logger.IsDebugEnabled()})
		return
	}

	if r.Method == http.MethodPost {
		var payload struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
			logger.SetDebug(payload.Enabled)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "success", "debug": logger.IsDebugEnabled()})
		return
	}
	
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// ---------------------------------------------------------
// 节点 (Providers) API
// ---------------------------------------------------------

func (h *AdminHandler) HandleNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		providers, err := h.providerRepo.GetUserProviders(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(providers)

	case http.MethodPost:
		var p domain.UserProvider
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := h.providerRepo.CreateUserProvider(r.Context(), &p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		go h.reloadAll(context.Background())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "success", "id": p.ID})

	case http.MethodPut:
		var p domain.UserProvider
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := h.providerRepo.UpdateUserProvider(r.Context(), &p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		go h.reloadAll(context.Background())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "success"})

	case http.MethodDelete:
		idStr := r.URL.Query().Get("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := h.providerRepo.DeleteUserProvider(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		go h.reloadAll(context.Background())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------
// 系统字典 (Sys Dictionary) API
// ---------------------------------------------------------

func (h *AdminHandler) HandleSysProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	providers, err := h.providerRepo.GetAllSysProviders(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	endpoints, err := h.providerRepo.GetAllSysAccessEndpoints(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	res := map[string]interface{}{
		"providers": providers,
		"endpoints": endpoints,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// ---------------------------------------------------------
// 模型 (Models) API
// ---------------------------------------------------------

func (h *AdminHandler) HandleModels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		models, err := h.modelRepo.GetUserModels(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(models)

	case http.MethodPost:
		// 手动为渠道添加模型（主要用于本地模型如 Ollama）
		var m domain.UserModel
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if m.UserProviderID <= 0 || m.ModelID == "" {
			http.Error(w, "user_provider_id and model_id are required", http.StatusBadRequest)
			return
		}
		if m.CapabilityTier == "" {
			m.CapabilityTier = "smart"
		}
		if err := h.modelRepo.CreateUserModel(r.Context(), &m); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		go h.reloadAll(context.Background())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "success", "id": m.ID})

	case http.MethodPut:
		var payload struct {
			ID             int    `json:"id"`
			CapabilityTier string `json:"capability_tier"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if payload.ID <= 0 || payload.CapabilityTier == "" {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		if err := h.modelRepo.UpdateUserModelTier(r.Context(), payload.ID, payload.CapabilityTier); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		go h.reloadAll(context.Background())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"msg":    "Model capability tier updated",
		})

	case http.MethodDelete:
		idStr := r.URL.Query().Get("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := h.modelRepo.DeleteUserModel(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		go h.reloadAll(context.Background())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *AdminHandler) SyncModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx := r.Context()

	providers, err := h.providerRepo.GetUserProviders(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	inferer := router.NewIntentInferer(h.intentRepo)
	var totalSynced int

	for _, p := range providers {
		if p.Status != 1 || p.BaseURL == "" {
			continue
		}

		client := &http.Client{Timeout: 10 * time.Second}
		req, err := http.NewRequestWithContext(ctx, "GET", p.BaseURL+"/v1/models", nil)
		if err != nil {
			continue
		}

		var creds map[string]string
		if err := json.Unmarshal(p.AuthCredentials, &creds); err == nil && creds["api_key"] != "" {
			req.Header.Set("Authorization", "Bearer "+creds["api_key"])
		}

		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != 200 {
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}

		var res struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		for _, m := range res.Data {
			weight := inferer.ParseVersionWeight(m.ID)
			tier := inferer.InferUnknownModel(ctx, m.ID)
			isLegacy := inferer.IsLegacyModel(m.ID)

			sysModel := &domain.SysModel{
				ModelID:       m.ID,
				DisplayName:   m.ID,
				VersionWeight: weight,
				IsLegacy:      isLegacy,
				CapabilityTier: tier,
			}
			_ = h.modelRepo.UpsertSysModel(ctx, sysModel)
			
			pm := &domain.SysProviderModel{
				ProviderID:    p.ProviderID,
				ModelID:       m.ID,
				ActualModelID: m.ID,
			}
			_ = h.modelRepo.UpsertSysProviderModel(ctx, pm)
			totalSynced++
		}
		
		// 简单处理 legacy 状态：同系列中有版本号更高的，老的就标 legacy
		// 这里暂不展开复杂的字串比较，仅做演示/占位
	}

	go h.reloadAll(context.Background())

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"synced": totalSynced,
	})
}

// ---------------------------------------------------------
// 意图字典 (Intents) API — 极简模式专用
// ---------------------------------------------------------

// HandleIntents 管理 user_model_intent_dict 表，即用户自定义意图覆盖（极简模式下使用）。
// GET    → 返回所有用户手动配置的意图条目（source='manual'）
// POST   → 新增或覆盖一条意图映射
// DELETE → 删除指定的手动意图映射
func (h *AdminHandler) HandleIntents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		intents, err := h.intentRepo.GetAllUserIntents(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// 转换为列表格式
		type IntentItem struct {
			ModelID        string `json:"model_id"`
			CapabilityTier string `json:"capability_tier"`
		}
		var list []IntentItem
		for k, v := range intents {
			list = append(list, IntentItem{ModelID: k, CapabilityTier: v})
		}
		if list == nil {
			list = []IntentItem{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)

	case http.MethodPost:
		var payload struct {
			ModelID        string `json:"model_id"`
			CapabilityTier string `json:"capability_tier"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if payload.ModelID == "" || payload.CapabilityTier == "" {
			http.Error(w, "model_id and capability_tier are required", http.StatusBadRequest)
			return
		}
		validTiers := map[string]bool{"smart": true, "fast": true, "reasoning": true}
		if !validTiers[payload.CapabilityTier] {
			http.Error(w, "capability_tier must be one of: smart, fast, reasoning", http.StatusBadRequest)
			return
		}
		intent := &domain.UserModelIntentDict{
			ModelID:        payload.ModelID,
			CapabilityTier: payload.CapabilityTier,
			Source:         "manual",
		}
		if err := h.intentRepo.SaveUserIntent(r.Context(), intent); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		go h.reloadPipeline(context.Background())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))

	case http.MethodDelete:
		modelID := r.URL.Query().Get("model")
		if modelID == "" {
			http.Error(w, "model query parameter is required", http.StatusBadRequest)
			return
		}
		if err := h.intentRepo.DeleteUserIntent(r.Context(), modelID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		go h.reloadPipeline(context.Background())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------
// 路由 (Routes) API
// ---------------------------------------------------------

func (h *AdminHandler) HandleRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		routes, err := h.routeRepo.GetAllUserCustomRoutes(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if routes == nil {
			routes = []domain.UserCustomRoute{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(routes)

	case http.MethodPost:
		var rt domain.UserCustomRoute
		if err := json.NewDecoder(r.Body).Decode(&rt); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := h.routeRepo.CreateUserCustomRoute(r.Context(), &rt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		go h.reloadPipeline(context.Background())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "success", "id": rt.ID})

	case http.MethodPut:
		var rt domain.UserCustomRoute
		if err := json.NewDecoder(r.Body).Decode(&rt); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := h.routeRepo.UpdateUserCustomRoute(r.Context(), &rt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		go h.reloadPipeline(context.Background())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "success"})

	case http.MethodDelete:
		idStr := r.URL.Query().Get("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := h.routeRepo.DeleteUserCustomRoute(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		go h.reloadPipeline(context.Background())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------
// OAuth 2.0 API
// ---------------------------------------------------------

func (h *AdminHandler) StartGoogleOAuth(w http.ResponseWriter, r *http.Request) {
	clientID, _ := h.settingsRepo.GetSetting(r.Context(), "google_oauth_client_id")
	clientSecret, _ := h.settingsRepo.GetSetting(r.Context(), "google_oauth_client_secret")

	if clientID == "" || clientSecret == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<script>alert("请先在系统设置中配置 Google OAuth Client ID 和 Secret。"); window.close();</script>`))
		return
	}

	state := fmt.Sprintf("%x", os.Getpid()) + strconv.FormatInt(int64(os.Getpid()), 16) // simple random state
	oauthStateStore.Store(state, map[string]string{
		"client_id":     clientID,
		"client_secret": clientSecret,
	})

	redirectURI := "http://127.0.0.1:27777/api/admin/oauth/google/callback"
	authURL := fmt.Sprintf("https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&access_type=offline&prompt=consent&state=%s",
		url.QueryEscape(clientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape("https://www.googleapis.com/auth/cloud-platform"),
		url.QueryEscape(state),
	)

	http.Redirect(w, r, authURL, http.StatusFound)
}

func (h *AdminHandler) CallbackGoogleOAuth(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	v, ok := oauthStateStore.LoadAndDelete(state)
	if !ok {
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}
	creds := v.(map[string]string)

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", url.Values{
		"code":          {code},
		"client_id":     {creds["client_id"]},
		"client_secret": {creds["client_secret"]},
		"redirect_uri":  {"http://127.0.0.1:27777/api/admin/oauth/google/callback"},
		"grant_type":    {"authorization_code"},
	})
	if err != nil || resp.StatusCode != 200 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<script>alert("获取 Token 失败。"); window.close();</script>`))
		return
	}
	defer resp.Body.Close()

	var tokenRes map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&tokenRes); err != nil {
		http.Error(w, "Failed to parse token response", http.StatusInternalServerError)
		return
	}

	refreshToken, ok := tokenRes["refresh_token"].(string)
	if !ok {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<script>alert("未获取到 refresh_token，请尝试重新登录。"); window.close();</script>`))
		return
	}

	adcJson := map[string]string{
		"client_id":     creds["client_id"],
		"client_secret": creds["client_secret"],
		"refresh_token": refreshToken,
		"type":          "authorized_user",
	}
	adcBytes, _ := json.MarshalIndent(adcJson, "", "  ")

	html := fmt.Sprintf(`<html><body><script>
		window.opener.postMessage({ type: 'google_adc_auth', data: %s }, '*');
		window.close();
	</script></body></html>`, strconv.Quote(string(adcBytes)))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

// ---------------------------------------------------------
// 注册总线
// ---------------------------------------------------------

// RegisterRoutes 为所有的接口注册路由
func (h *AdminHandler) RegisterRoutes(mux *http.ServeMux) {
	// 系统基本信息
	mux.HandleFunc("/api/admin/info", h.GetInfo)

	// 核心业务
	mux.HandleFunc("/api/admin/nodes", h.HandleNodes)
	mux.HandleFunc("/api/admin/sys_providers", h.HandleSysProviders)
	mux.HandleFunc("/api/admin/models", h.HandleModels)
	mux.HandleFunc("/api/admin/models/sync", h.SyncModels)
	mux.HandleFunc("/api/admin/models/sync-global", h.SyncGlobalModels)
	mux.HandleFunc("/api/admin/routes", h.HandleRoutes)
	// 意图映射管理（极简模式专用）
	mux.HandleFunc("/api/admin/intents", h.HandleIntents)

	// 边缘存根 API
	mux.HandleFunc("/api/stats", h.GetStats)
	mux.HandleFunc("/api/admin/settings", h.HandleSettings)
	mux.HandleFunc("/api/admin/clients/status", h.GetClientsStatus)
	mux.HandleFunc("/api/admin/clients/apply", h.ApplyClientConfig)
	mux.HandleFunc("/api/admin/clients/restore", h.RestoreClientConfig)
	mux.HandleFunc("/api/admin/logs", h.GetLogs)
	mux.HandleFunc("/api/admin/debug", h.SetDebug)

	// OAuth 2.0 API
	mux.HandleFunc("/api/admin/oauth/google/start", h.StartGoogleOAuth)
	mux.HandleFunc("/api/admin/oauth/google/callback", h.CallbackGoogleOAuth)
}


// SyncGlobalModels 从全网同步最新的开源/公开模型库字典
func (h *AdminHandler) SyncGlobalModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	inferer := router.NewIntentInferer(h.intentRepo)
	syncService := modelsync.NewSyncService(h.modelRepo, inferer)

	go func() {
		_ = syncService.SyncGlobalModels(context.Background())
	}()

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"success","message":"Global model sync started in background"}`))
}
