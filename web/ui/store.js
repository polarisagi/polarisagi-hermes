const { ref, reactive } = Vue;

export const state = reactive({
    lang: window.getSystemLanguage(),
    theme: window.getSystemTheme(),
    currentTab: 'dashboard',
    toast: { show: false, message: '', type: 'success' },
    apiData: [],
    concurrency: { active: 0, waiting: 0, max: 0 },
    nodes: [],
    routes: [],
    allModels: [],
    settings: {
        listen_addr: '127.0.0.1:28888',
        breaker: {
            initial_cooldown_seconds: 60,
            max_cooldown_seconds: 3600,
            failure_threshold: 3,
            failure_window_seconds: 120
        },
        google_oauth_client_id: '',
        google_oauth_client_secret: ''
    },
    version: '',
    latestVersion: '',
    updateAvailable: false,
    isUpdating: false,
    debugEnabled: false,
    logLevelFilter: 'all',
    logsText: 'Loading logs...',
    isAutoScroll: true,
    availableAccounts: [],
    selectedAccount: 'all',
    activePreset: 'today',
    startDate: '',
    endDate: ''
});

export const t = (key) => window.messages[state.lang][key] || key;

export const showToast = (msg, type = 'success') => {
    state.toast = { show: true, message: msg, type };
    setTimeout(() => { state.toast.show = false }, 3000);
};

export const formatNum = (num) => Number(num || 0).toFixed(4);
export const formatToken = (num) => new Intl.NumberFormat().format(num);
export const formatShortDate = (dt) => dt ? dt.split('T')[0].split(' ')[0] : '-';
export const successRateColor = (rate) => rate > 95 ? 'border-emerald-500' : (rate > 80 ? 'border-yellow-500' : 'border-red-500');

export const protocolLabel = (p) => {
    const labels = { openai: 'OpenAI', google: 'Google Agent Platform', anthropic: 'Anthropic' };
    return labels[p] || p;
};
export const protocolClass = (p) => {
    const classes = { openai: 'text-indigo-400', google: 'text-emerald-400', anthropic: 'text-orange-400' };
    return classes[p] || 'text-slate-400';
};
export const protocolBadge = (p) => {
    const badges = { openai: 'bg-indigo-600 border-indigo-500/50', google: 'bg-emerald-600 border-emerald-500/50', anthropic: 'bg-orange-600 border-orange-500/50' };
    return badges[p] || 'bg-slate-600 border-slate-500/50';
};

export const setLang = (l) => {
    window.setSystemLanguage(l);
    state.lang = l;
};

export const setTheme = (t) => {
    state.theme = t;
    window.applyTheme(t);
};

export const checkForUpdates = (currentVer) => {
    if (currentVer === 'dev' || !currentVer.startsWith('v')) return;
    fetch('https://api.github.com/repos/mrlaoliai/polaris-gateway/releases/latest')
        .then(r => r.json())
        .then(d => {
            if (d.tag_name && d.tag_name !== currentVer) {
                state.latestVersion = d.tag_name;
                state.updateAvailable = true;
            }
        }).catch(console.error);
};

export const triggerUpdate = () => {
    if (!confirm(state.lang === 'zh' ? `确定要平滑热更新到 ${state.latestVersion} 吗？\n整个过程完全自动化，并且不会中断正在处理的流量。` : `Are you sure you want to smooth update to ${state.latestVersion}?\nThe process is fully automated and will not interrupt active traffic.`)) return;
    
    state.isUpdating = true;
    fetch('/api/admin/update', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ target_version: state.latestVersion })
    }).then(r => r.json()).then(d => {
        showToast(d.message || t("update_started"), "success");
        setTimeout(() => {
            window.location.reload();
        }, 3500);
    }).catch(e => {
        showToast(t("update_failed"), "error");
        state.isUpdating = false;
    });
};
