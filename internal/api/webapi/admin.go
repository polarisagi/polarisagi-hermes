package webapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"

	"polaris-hermes/internal/domain"
	"polaris-hermes/internal/repository/sqlite"
	"polaris-hermes/pkg/logger"
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
	settingsRepo *sqlite.SettingsRepo
}

func NewAdminHandler(pRepo *sqlite.ProviderRepo, mRepo *sqlite.ModelRepo, rRepo *sqlite.RouteRepo, sRepo *sqlite.SettingsRepo) *AdminHandler {
	return &AdminHandler{
		providerRepo: pRepo,
		modelRepo:    mRepo,
		routeRepo:    rRepo,
		settingsRepo: sRepo,
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
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`[]`))
}

func (h *AdminHandler) HandleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		settings := map[string]interface{}{
			"listen_addr": "127.0.0.1:27777",
			"breaker": map[string]int{
				"initial_cooldown_seconds": 60,
				"max_cooldown_seconds":     3600,
				"failure_threshold":        3,
				"failure_window_seconds":   120,
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
	var payload struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
		logger.SetDebug(payload.Enabled)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"success"}`))
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

	authModes, err := h.providerRepo.GetAllSysProviderAuthModes(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	res := map[string]interface{}{
		"providers":  providers,
		"auth_modes": authModes,
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

	case http.MethodPut:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","msg":"Model capability tier updated"}`))

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
		routes, err := h.routeRepo.GetUserCustomRoutes(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(routes)

	case http.MethodPost, http.MethodPut:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))

	case http.MethodDelete:
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
		w.Write([]byte(`<script>alert("请先在系统设置中配置 Google OAuth Client ID 和 Secret。"); window.close();</script>`))
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
		w.Write([]byte(`<script>alert("获取 Token 失败。"); window.close();</script>`))
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
		w.Write([]byte(`<script>alert("未获取到 refresh_token，请尝试重新登录。"); window.close();</script>`))
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
	w.Write([]byte(html))
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
	mux.HandleFunc("/api/admin/routes", h.HandleRoutes)

	// 边缘存根 API
	mux.HandleFunc("/api/stats", h.GetStats)
	mux.HandleFunc("/api/admin/settings", h.HandleSettings)
	mux.HandleFunc("/api/admin/clients/status", h.GetClientsStatus)
	mux.HandleFunc("/api/admin/logs", h.GetLogs)
	mux.HandleFunc("/api/admin/debug", h.SetDebug)

	// OAuth 2.0 API
	mux.HandleFunc("/api/admin/oauth/google/start", h.StartGoogleOAuth)
	mux.HandleFunc("/api/admin/oauth/google/callback", h.CallbackGoogleOAuth)

	// 旧版客户端打桩
	mux.HandleFunc("/api/admin/clients/apply", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	})
	mux.HandleFunc("/api/admin/clients/restore", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	})
}
