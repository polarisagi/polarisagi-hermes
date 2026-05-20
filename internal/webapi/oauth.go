package webapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"polaris-gateway/internal/config"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	// gcloud CLI 内置的公共 Client ID 和 Secret（Desktop App）
	// 注意：Google 现在对第三方程序使用 gcloud Client ID 做严格来源检查，可能被直接拦截
	// 用户如果遇到"此应用已被阻止"错误，请在 Google Cloud Console 创建自己的 OAuth 客户端，
	// 并将 Client ID 和 Secret 填入系统设置
	gcloudClientID     = "764086051850-6qr4p6gpi6hn506pt8ejuq83di341hur.apps.googleusercontent.com"
	gcloudClientSecret = "d-qshared-pb-base"
)

func getOAuthCredentials() (clientID, clientSecret string) {
	clientID = config.AppConfig.GoogleOAuthClientID
	clientSecret = config.AppConfig.GoogleOAuthClientSecret
	if clientID == "" {
		clientID = gcloudClientID
	}
	if clientSecret == "" {
		clientSecret = gcloudClientSecret
	}
	return
}

var (
	oauthStates sync.Map
)

func getOAuthConfig(r *http.Request) *oauth2.Config {
	host := r.Host
	if !strings.Contains(host, ":") {
		host = host + ":80"
	}
	
	parts := strings.Split(host, ":")
	redirectURL := fmt.Sprintf("http://127.0.0.1:%s/api/admin/oauth/google/callback", parts[1])

	clientID, clientSecret := getOAuthCredentials()

	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/cloud-platform",
			"https://www.googleapis.com/auth/generative-language.retriever",
		},
		Endpoint: google.Endpoint,
	}
}

func generateStateOauthCookie(w http.ResponseWriter) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	state := base64.URLEncoding.EncodeToString(b)
	
	oauthStates.Store(state, time.Now().Add(10*time.Minute))
	
	go func() {
		oauthStates.Range(func(key, value interface{}) bool {
			if time.Now().After(value.(time.Time)) {
				oauthStates.Delete(key)
			}
			return true
		})
	}()

	return state
}

func AdminOAuthGoogleStartHandler(w http.ResponseWriter, r *http.Request) {
	state := generateStateOauthCookie(w)
	conf := getOAuthConfig(r)
	url := conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func AdminOAuthGoogleCallbackHandler(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if _, ok := oauthStates.Load(state); !ok {
		http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
		return
	}
	oauthStates.Delete(state)

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Code not found", http.StatusBadRequest)
		return
	}

	conf := getOAuthConfig(r)
	tok, err := conf.Exchange(context.Background(), code)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to exchange token: %v", err), http.StatusInternalServerError)
		return
	}

	clientID, clientSecret := getOAuthCredentials()

	adc := map[string]interface{}{
		"client_id":     clientID,
		"client_secret": clientSecret,
		"refresh_token": tok.RefreshToken,
		"type":          "authorized_user",
	}

	jsonBytes, err := json.MarshalIndent(adc, "", "  ")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to marshal JSON: %v", err), http.StatusInternalServerError)
		return
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">
	<title>授权成功</title>
	<style>
		body { font-family: system-ui, -apple-system, sans-serif; background: #f3f4f6; padding: 2rem; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; }
		.card { background: white; padding: 2rem; border-radius: 8px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); max-width: 600px; width: 100%%; text-align: center; }
		h1 { color: #10b981; margin-top: 0; }
		textarea { width: 100%%; height: 200px; padding: 0.5rem; border: 1px solid #d1d5db; border-radius: 4px; font-family: monospace; font-size: 14px; margin: 1rem 0; resize: vertical; }
		button { background: #3b82f6; color: white; border: none; padding: 0.5rem 1rem; border-radius: 4px; cursor: pointer; font-size: 16px; transition: background 0.2s; }
		button:hover { background: #2563eb; }
		.success-msg { color: #10b981; margin-top: 1rem; display: none; }
	</style>
</head>
<body>
	<div class="card">
		<h1>✅ 授权成功</h1>
		<p>Google ADC 凭证已生成。</p>
		<textarea id="jsonContent" readonly>%s</textarea>
		<br>
		<button onclick="copyAndClose()">复制凭证并关闭</button>
		<div id="msg" class="success-msg">已复制到剪贴板！窗口将自动关闭...</div>
	</div>
	<script>
		// Try to send to parent if opened as popup
		if (window.opener) {
			window.opener.postMessage({ type: 'google_adc_auth', data: document.getElementById('jsonContent').value }, '*');
			setTimeout(() => window.close(), 1000);
		}

		function copyAndClose() {
			const textarea = document.getElementById('jsonContent');
			textarea.select();
			document.execCommand('copy');
			document.getElementById('msg').style.display = 'block';
			if (window.opener) {
				setTimeout(() => window.close(), 1000);
			}
		}
	</script>
</body>
</html>`, string(jsonBytes))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}
