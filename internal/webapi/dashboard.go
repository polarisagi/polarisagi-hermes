package webapi

import (
	"io/fs"
	"log/slog"
	"net/http"

	"polaris-gateway/web"
)

// DashboardHandler 返回用于服务前端 UI 的静态文件处理器
func DashboardHandler() http.Handler {
	uiSub, err := fs.Sub(web.FS, "ui")
	if err != nil {
		slog.Error("前端 UI 静态文件系统挂载失败", "error", err)
		return http.NotFoundHandler()
	}
	
	fileServer := http.FileServer(http.FS(uiSub))
	return http.StripPrefix("/dashboard/", fileServer)
}
