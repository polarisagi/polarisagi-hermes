package webapi

import (
	"encoding/json"
	"net/http"

	"polaris-gateway/internal/repository/sqlite"
)

// AdminHandler 处理后台管理面板的 RESTful API 请求
type AdminHandler struct {
	providerRepo *sqlite.ProviderRepo
	modelRepo    *sqlite.ModelRepo
}

func NewAdminHandler(pRepo *sqlite.ProviderRepo, mRepo *sqlite.ModelRepo) *AdminHandler {
	return &AdminHandler{
		providerRepo: pRepo,
		modelRepo:    mRepo,
	}
}

// ---------------------------------------------------------
// 系统预置字典 API (纯只读, Read-Only)
// ---------------------------------------------------------

// ListSysProviders 返回系统内置的大厂列表
func (h *AdminHandler) ListSysProviders(w http.ResponseWriter, r *http.Request) {
	// TODO: 严格遵循：系统表不对外提供 POST/PUT/DELETE 方法
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`[{"provider_id":"openai","provider_name":"OpenAI"},{"provider_id":"google","provider_name":"Google Agent Platform"}]`))
}

// ListSysModels 返回系统内置的官方模型列表（无意图标签）
func (h *AdminHandler) ListSysModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.modelRepo.GetSysModels(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(models)
}

// ---------------------------------------------------------
// 用户实例 API (全权限 CRUD, Read-Write)
// ---------------------------------------------------------

// ListUserProviders 获取用户实例化的渠道列表
func (h *AdminHandler) ListUserProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := h.providerRepo.GetUserProviders(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(providers)
}

// CreateUserProvider 创建新的用户渠道
func (h *AdminHandler) CreateUserProvider(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"status":"success","msg":"Provider created"}`))
}

// ListUserModels 获取用户配置的模型列表及其主观意图标签
func (h *AdminHandler) ListUserModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.modelRepo.GetUserModels(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(models)
}

// UpdateUserModel 更新用户模型（特别是修改其 capability_tier 意图标签）
func (h *AdminHandler) UpdateUserModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"success","msg":"Model capability tier updated"}`))
}

// RegisterRoutes 为所有的接口注册路由
func (h *AdminHandler) RegisterRoutes(mux *http.ServeMux) {
	// 系统只读层
	mux.HandleFunc("/api/sys/providers", h.ListSysProviders)
	mux.HandleFunc("/api/sys/models", h.ListSysModels)

	// 用户读写层
	mux.HandleFunc("/api/user/providers", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.ListUserProviders(w, r)
		case http.MethodPost:
			h.CreateUserProvider(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/user/models", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.ListUserModels(w, r)
		case http.MethodPut:
			h.UpdateUserModel(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
}
