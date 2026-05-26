package webapi

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"

	"polaris-hermes/internal/domain"
	"polaris-hermes/internal/repository/sqlite"
	"polaris-hermes/pkg/logger"
)

var Version = "v2.0.0"

// AdminHandler 处理后台管理面板的 RESTful API 请求
type AdminHandler struct {
	providerRepo *sqlite.ProviderRepo
	modelRepo    *sqlite.ModelRepo
	routeRepo    *sqlite.RouteRepo
}

func NewAdminHandler(pRepo *sqlite.ProviderRepo, mRepo *sqlite.ModelRepo, rRepo *sqlite.RouteRepo) *AdminHandler {
	return &AdminHandler{
		providerRepo: pRepo,
		modelRepo:    mRepo,
		routeRepo:    rRepo,
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

func (h *AdminHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{}`))
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
	mux.HandleFunc("/api/admin/settings", h.GetSettings)
	mux.HandleFunc("/api/admin/clients/status", h.GetClientsStatus)
	mux.HandleFunc("/api/admin/logs", h.GetLogs)
	mux.HandleFunc("/api/admin/debug", h.SetDebug)

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
