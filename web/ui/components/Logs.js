import { state, t, showToast } from '../store.js';

export default {
    name: 'Logs',
    setup() {
        const fetchLogs = async () => {
            if (state.currentTab !== 'logs') return;
            try {
                const res = await fetch('/api/admin/logs');
                const rawText = await res.text();
                let lines = rawText.split('\n');
                if (state.logLevelFilter !== 'all') {
                    const levelStr = `level=${state.logLevelFilter.toUpperCase()}`;
                    lines = lines.filter(line => line.includes(levelStr) || line.trim() === '');
                }
                state.logsText = lines.join('\n');
                
                if (state.isAutoScroll) {
                    Vue.nextTick(() => {
                        const container = document.getElementById('logContainer');
                        if (container) container.scrollTop = container.scrollHeight;
                    });
                }
            } catch (e) {
                console.error("Fetch logs failed", e);
            }
        };

        const fetchDebug = async () => {
            try {
                const res = await fetch('/api/admin/debug');
                const json = await res.json();
                state.debugEnabled = json.debug;
            } catch (e) { console.error(e) }
        };

        const toggleDebug = async () => {
            try {
                const newVal = !state.debugEnabled;
                const res = await fetch('/api/admin/debug', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ enabled: newVal })
                });
                const json = await res.json();
                state.debugEnabled = json.debug;
                showToast(state.debugEnabled ? t('debug_enabled') : t('debug_disabled'));
            } catch(e) {
                showToast(t("debug_switch_failed"), 'error');
            }
        };

        let logInterval = null;

        Vue.onMounted(() => {
            fetchDebug();
            fetchLogs();
            logInterval = setInterval(() => {
                fetchLogs();
            }, 3000);
        });

        Vue.onUnmounted(() => {
            if (logInterval) clearInterval(logInterval);
        });

        return {
            state,
            t,
            fetchLogs,
            toggleDebug
        };
    },
    template: `
            <div v-show="state.currentTab === 'logs'" class="max-w-6xl mx-auto flex flex-col h-full">
                <div class="flex justify-between items-center mb-6">
                    <h2 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t("tab_logs_title") }}</h2>
                    <div class="flex gap-3">
                        <select v-model="state.logLevelFilter" @change="fetchLogs" class="bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-700 text-gray-700 dark:text-slate-300 text-sm rounded-lg block p-2 outline-none focus:ring-2 focus:ring-blue-500">
                            <option value="all">{{ t("log_level_all_desc") }}</option>
                            <option value="info">INFO</option>
                            <option value="warn">WARN</option>
                            <option value="error">ERROR</option>
                            <option value="debug">DEBUG</option>
                        </select>
                        <button @click="toggleDebug" :class="state.debugEnabled ? 'bg-purple-600 text-white border-purple-500' : 'bg-white dark:bg-slate-800 text-gray-500 dark:text-slate-400 border-gray-300 dark:border-slate-700'" class="px-4 py-2 rounded-lg text-sm border transition flex items-center gap-2">
                            <span>🐛</span>
                            <span>{{ state.debugEnabled ? 'DEBUG ON' : 'DEBUG OFF' }}</span>
                        </button>
                        <button @click="state.isAutoScroll = !state.isAutoScroll" :class="state.isAutoScroll ? 'bg-emerald-600/20 text-emerald-400 border-emerald-500/50' : 'bg-white dark:bg-slate-800 text-gray-500 dark:text-slate-400 border-gray-300 dark:border-slate-700'" class="px-4 py-2 rounded-lg text-sm border transition flex items-center gap-2">
                            <span v-if="state.isAutoScroll" class="relative flex h-2 w-2"><span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span><span class="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span></span>
                            <span>{{ t("auto_scroll_text") }} {{ state.isAutoScroll ? 'ON' : 'OFF' }}</span>
                        </button>
                        <button @click="fetchLogs" class="bg-white dark:bg-slate-800 hover:bg-gray-100 dark:hover:bg-slate-700 text-gray-700 dark:text-white px-4 py-2 rounded-lg transition text-sm flex items-center gap-2 border border-gray-300 dark:border-slate-700">
                            {{ t("btn_refresh_logs") }}
                        </button>
                    </div>
                </div>
                
                <div class="card rounded-xl shadow-lg flex-1 overflow-hidden flex flex-col bg-gray-50 dark:bg-[#0d1117] border border-gray-200 dark:border-[#30363d]">
                    <div class="bg-gray-100 dark:bg-[#161b22] border-b border-gray-200 dark:border-[#30363d] px-4 py-2 flex items-center gap-2">
                        <div class="flex gap-1.5">
                            <div class="w-3 h-3 rounded-full bg-red-500/80"></div>
                            <div class="w-3 h-3 rounded-full bg-yellow-500/80"></div>
                            <div class="w-3 h-3 rounded-full bg-emerald-500/80"></div>
                        </div>
                        <span class="text-gray-500 dark:text-[#8b949e] text-xs font-mono ml-2">polaris-gateway.log</span>
                    </div>
                    <div class="flex-1 overflow-auto p-4 font-mono text-[13px] leading-relaxed text-gray-800 dark:text-[#c9d1d9]" id="logContainer">
                        <pre class="whitespace-pre-wrap break-all">{{ state.logsText }}</pre>
                    </div>
                </div>
            </div>
    `
};