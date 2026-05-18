-- 004: 添加 Google OAuth 2.0 Client ID / Secret 配置字段
-- 用户可在 Google Cloud Console 创建 OAuth 2.0 桌面应用凭据后填入
-- 留空则回退使用 gcloud 内置 Client ID（可能被 Google 拦截）
ALTER TABLE sys_settings ADD COLUMN google_oauth_client_id TEXT DEFAULT '';
ALTER TABLE sys_settings ADD COLUMN google_oauth_client_secret TEXT DEFAULT '';
