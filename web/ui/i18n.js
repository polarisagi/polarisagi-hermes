const messages = {
    zh: {
        // Sidebar
        app_title: "Polaris",
        app_subtitle: "Gateway Admin",
        new_version_found: "✨ 发现新版本",
        updating: "正在自动更新...",
        tab_dashboard: "📊 监控大盘",
        tab_nodes: "🌐 节点管理",
        tab_routes: "🔀 路由管理",
        tab_settings: "⚙️ 系统设置",
        tab_logs: "📝 实时日志",
        
        // Common
        cancel: "取消",
        save: "保存",
        edit: "编辑",
        delete: "删除",
        confirm_delete: "确定要删除吗？",
        status_enabled: "🟢 已启用",
        status_disabled: "⚪️ 已禁用",
        status_exhausted: "🔴 熔断/过期",
        all: "全部",
        all_summary: "全部汇总",
        network_error: "网络错误",
        save_failed: "保存失败",
        delete_failed: "删除失败",
        
        // Dashboard
        overall_trend: "全局趋势图",
        model_distribution: "模型占比",
        cost_trend: "按天消费统计 (USD)",
        filter_today: "今天",
        filter_7days: "近7天",
        filter_30days: "近30天",
        filter_all_time: "全部时间",
        total_requests: "总请求数",
        total_cost: "总计费",
        cost_unit: "美元",
        concurrent_requests: "当前并发请求数",
        active_requests: "处理中",
        waiting_requests: "排队中",
        peak_requests: "峰值",
        realtime_status: "系统实时状态",
        
        // Nodes
        add_node: "+ 添加节点",
        provider: "协议类型",
        node_name: "节点名称",
        credentials: "凭据",
        priority: "优先级",
        status: "运行状态",
        balance: "总额度",
        used_amount: "已用",
        limit_percent: "熔断水位线",
        valid_range: "有效期",
        actions: "操作",
        base_url: "Base URL",
        project_id: "GCP Project ID",
        location: "Location",
        node_updated: "节点已更新",
        node_added: "节点已添加",
        node_deleted: "节点已删除",
        err_empty_node: "节点名称和 API Key 不能为空",
        err_gcp_project: "Google Agent Platform 节点必须填写 GCP Project ID",
        err_negative_numbers: "优先级、额度等数字不能为负数",
        err_limit_exceed: "阻断水位线不能超过100",
        adc_filled: "✅ Google ADC 凭证已自动填入",
        
        // Routes
        add_route: "+ 添加路由",
        source_protocol: "源协议",
        target_protocol: "目标协议",
        route_desc: "路由说明",
        model_mappings: "模型映射",
        match_rule: "匹配规则",
        target_model: "目标模型",
        add_mapping: "+ 增加映射",
        exact_match: "精确匹配/前缀通配",
        route_updated: "路由已更新",
        route_added: "路由已添加",
        route_deleted: "路由已删除",
        err_empty_mapping: "至少需要填写一个模型匹配规则",
        err_empty_protocols: "必须选择源协议和目标协议",
        
        // Route types & descs
        route_anthropic_direct: "Anthropic — 透传直通",
        route_anthropic_google: "Google Agent Platform — Gemini / GEAP Claude",
        route_anthropic_openai: "OpenAI — 协议转换",
        route_openai_direct: "OpenAI — 透传直通",
        route_openai_google: "Google Agent Platform — Vertex OAI 兼容端点",
        route_google_direct: "Google Agent Platform — 透传直通",
        
        desc_anthropic_direct: "透传直通 · Anthropic 账号多节点轮询，请求格式不变",
        desc_anthropic_google: "Anthropic 格式转 Gemini 原生协议；claude-* 模型走 GEAP rawPredict 直通",
        desc_anthropic_openai: "Anthropic Messages 格式转 OpenAI Chat Completions 格式",
        desc_openai_direct: "透传直通 · OpenAI 账号多节点轮询，请求格式不变",
        desc_openai_google: "OpenAI 格式转 Vertex AI OpenAI 兼容端点 (endpoints/openapi)",
        desc_google_direct: "透传直通 · Google 账号多节点轮询，请求格式不变",
        
        // Settings
        listen_addr: "监听地址",
        listen_addr_desc: "重启生效，默认 127.0.0.1:28888",
        google_oauth: "Google OAuth 全局配置 (可选)",
        google_oauth_desc: "如果不填，则使用各个节点独立配置的 ADC JSON。",
        client_id: "Client ID",
        client_secret: "Client Secret",
        circuit_breaker: "熔断器配置",
        cb_initial_cooldown: "初始冷却时间 (秒)",
        cb_max_cooldown: "最大冷却时间 (秒)",
        cb_failure_threshold: "失败阈值 (次)",
        cb_failure_window: "失败统计窗口 (秒)",
        settings_saved: "系统设置已保存并热加载生效",
        err_negative: "各项设置的值不能为负数",
        
        // Logs
        auto_scroll: "自动滚动",
        log_level_all: "所有日志",
        log_level_warn: "⚠️ 仅异常",
        debug_enabled: "Debug 模式已开启",
        debug_disabled: "Debug 模式已关闭",
        
        // JS specific
        confirm_update: "确定要平滑热更新到 {version} 吗？\n整个过程完全自动化，并且不会中断正在处理的流量。",
        confirm_delete_node: "确定要删除节点 {name} 吗？",
        confirm_delete_route: "确定要删除这条路由规则吗？",
    },
    en: {
        // Sidebar
        app_title: "Polaris",
        app_subtitle: "Gateway Admin",
        new_version_found: "✨ New Version",
        updating: "Updating...",
        tab_dashboard: "📊 Dashboard",
        tab_nodes: "🌐 Nodes",
        tab_routes: "🔀 Routes",
        tab_settings: "⚙️ Settings",
        tab_logs: "📝 Logs",
        
        // Common
        cancel: "Cancel",
        save: "Save",
        edit: "Edit",
        delete: "Delete",
        confirm_delete: "Are you sure you want to delete?",
        status_enabled: "🟢 Enabled",
        status_disabled: "⚪️ Disabled",
        status_exhausted: "🔴 Exhausted",
        all: "All",
        all_summary: "All Summary",
        network_error: "Network Error",
        save_failed: "Save Failed",
        delete_failed: "Delete Failed",
        
        // Dashboard
        overall_trend: "Overall Trend",
        model_distribution: "Model Distribution",
        cost_trend: "Daily Cost (USD)",
        filter_today: "Today",
        filter_7days: "Last 7 Days",
        filter_30days: "Last 30 Days",
        filter_all_time: "All Time",
        total_requests: "Total Requests",
        total_cost: "Total Cost",
        cost_unit: "USD",
        concurrent_requests: "Concurrent Requests",
        active_requests: "Active",
        waiting_requests: "Waiting",
        peak_requests: "Peak",
        realtime_status: "Realtime Status",
        
        // Nodes
        add_node: "+ Add Node",
        provider: "Provider",
        node_name: "Node Name",
        credentials: "Credentials",
        priority: "Priority",
        status: "Status",
        balance: "Balance",
        used_amount: "Used",
        limit_percent: "Limit %",
        valid_range: "Valid Range",
        actions: "Actions",
        base_url: "Base URL",
        project_id: "GCP Project ID",
        location: "Location",
        node_updated: "Node Updated",
        node_added: "Node Added",
        node_deleted: "Node Deleted",
        err_empty_node: "Node Name and API Key cannot be empty",
        err_gcp_project: "GCP Project ID is required for Google Agent Platform",
        err_negative_numbers: "Priority and balance cannot be negative",
        err_limit_exceed: "Limit percent cannot exceed 100",
        adc_filled: "✅ Google ADC Filled",
        
        // Routes
        add_route: "+ Add Route",
        source_protocol: "Source Protocol",
        target_protocol: "Target Protocol",
        route_desc: "Description",
        model_mappings: "Model Mappings",
        match_rule: "Match Rule",
        target_model: "Target Model",
        add_mapping: "+ Add Mapping",
        exact_match: "Exact / Prefix Match",
        route_updated: "Route Updated",
        route_added: "Route Added",
        route_deleted: "Route Deleted",
        err_empty_mapping: "At least one model mapping rule is required",
        err_empty_protocols: "Source and target protocols must be selected",
        
        // Route types & descs
        route_anthropic_direct: "Anthropic — Direct Passthrough",
        route_anthropic_google: "Google Agent Platform — Gemini / GEAP Claude",
        route_anthropic_openai: "OpenAI — Protocol Conversion",
        route_openai_direct: "OpenAI — Direct Passthrough",
        route_openai_google: "Google Agent Platform — Vertex OAI Compatible",
        route_google_direct: "Google Agent Platform — Direct Passthrough",
        
        desc_anthropic_direct: "Direct Passthrough · Anthropic multi-node round-robin, request format unchanged",
        desc_anthropic_google: "Convert Anthropic format to Gemini native protocol; claude-* models use GEAP rawPredict",
        desc_anthropic_openai: "Convert Anthropic Messages format to OpenAI Chat Completions format",
        desc_openai_direct: "Direct Passthrough · OpenAI multi-node round-robin, request format unchanged",
        desc_openai_google: "Convert OpenAI format to Vertex AI OpenAI compatible endpoint (endpoints/openapi)",
        desc_google_direct: "Direct Passthrough · Google multi-node round-robin, request format unchanged",
        
        // Settings
        listen_addr: "Listen Address",
        listen_addr_desc: "Requires restart. Default 127.0.0.1:28888",
        google_oauth: "Global Google OAuth (Optional)",
        google_oauth_desc: "If empty, node-specific ADC JSON will be used.",
        client_id: "Client ID",
        client_secret: "Client Secret",
        circuit_breaker: "Circuit Breaker",
        cb_initial_cooldown: "Initial Cooldown (s)",
        cb_max_cooldown: "Max Cooldown (s)",
        cb_failure_threshold: "Failure Threshold",
        cb_failure_window: "Failure Window (s)",
        settings_saved: "Settings saved and hot-reloaded successfully",
        err_negative: "Settings values cannot be negative",
        
        // Logs
        auto_scroll: "Auto Scroll",
        log_level_all: "All Logs",
        log_level_warn: "⚠️ Errors Only",
        debug_enabled: "Debug Mode Enabled",
        debug_disabled: "Debug Mode Disabled",
        
        // JS specific
        confirm_update: "Are you sure you want to smooth update to {version}?\nThe process is fully automated and will not interrupt active traffic.",
        confirm_delete_node: "Are you sure you want to delete node {name}?",
        confirm_delete_route: "Are you sure you want to delete this route?",
    }
};

function getSystemLanguage() {
    const lang = localStorage.getItem('polaris_lang');
    if (lang && ['zh', 'en'].includes(lang)) {
        return lang;
    }
    return navigator.language.startsWith('zh') ? 'zh' : 'en';
}

function setSystemLanguage(lang) {
    localStorage.setItem('polaris_lang', lang);
    window.location.reload();
}

function getSystemTheme() {
    const theme = localStorage.getItem('polaris_theme');
    if (theme && ['light', 'dark', 'system'].includes(theme)) {
        return theme;
    }
    return 'system';
}

function applyTheme(theme) {
    localStorage.setItem('polaris_theme', theme);
    if (theme === 'dark' || (theme === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
        document.documentElement.classList.add('dark');
    } else {
        document.documentElement.classList.remove('dark');
    }
}

// Watch for system theme changes
window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', e => {
    if (getSystemTheme() === 'system') {
        applyTheme('system');
    }
});

// Initial apply
applyTheme(getSystemTheme());
