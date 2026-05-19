import { state, t, showToast } from '../store.js';

export default {
    name: 'Settings',
    setup() {
        const fetchSettings = async () => {
            try {
                const res = await fetch('/api/admin/settings');
                state.settings = await res.json();
            } catch (e) { console.error(e) }
        };

        const saveSettings = async () => {
            if (state.settings.breaker.failure_threshold < 0 || 
                state.settings.breaker.failure_window_seconds < 0 || 
                state.settings.breaker.initial_cooldown_seconds < 0 || 
                state.settings.breaker.max_cooldown_seconds < 0) {
                showToast(t('err_negative'), 'error');
                return;
            }
            
            try {
                const payload = {
                    listen_addr: state.settings.listen_addr,
                    initial_cooldown_seconds: state.settings.breaker.initial_cooldown_seconds,
                    max_cooldown_seconds: state.settings.breaker.max_cooldown_seconds,
                    failure_threshold: state.settings.breaker.failure_threshold,
                    failure_window_seconds: state.settings.breaker.failure_window_seconds,
                    google_oauth_client_id: state.settings.google_oauth_client_id || '',
                    google_oauth_client_secret: state.settings.google_oauth_client_secret || ''
                };
                const res = await fetch('/api/admin/settings', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                if (res.ok) {
                    showToast(t('settings_saved'));
                } else {
                    showToast(t('save_failed'), 'error');
                }
            } catch(e) {
                showToast(t('network_error'), 'error');
            }
        };

        const resetSettings = () => {
            if(!confirm(state.lang === 'zh' ? '确定要恢复系统默认设置吗？' : 'Are you sure you want to reset to default settings?')) return;
            state.settings = {
                listen_addr: '127.0.0.1:28888',
                breaker: {
                    initial_cooldown_seconds: 60,
                    max_cooldown_seconds: 3600,
                    failure_threshold: 3,
                    failure_window_seconds: 120
                },
                google_oauth_client_id: '',
                google_oauth_client_secret: ''
            };
        };

        const redirectUri = window.location.protocol + '//' + window.location.host + '/api/admin/oauth/google/callback';

        Vue.onMounted(() => {
            fetchSettings();
        });

        return {
            state,
            t,
            saveSettings,
            resetSettings,
            redirectUri
        };
    },
    template: `
            <div v-show="state.currentTab === 'settings'" class="max-w-3xl mx-auto">
                <h2 class="text-2xl font-bold text-gray-900 dark:text-white mb-6">{{ t("tab_settings_title") }}</h2>
                
                <div class="card rounded-xl p-6 shadow-lg space-y-6">
                    <div>
                        <h3 class="text-lg font-medium text-gray-900 dark:text-white mb-4 border-b border-gray-300 dark:border-slate-700 pb-2">{{ t("settings_basic") }}</h3>
                        <div class="space-y-4">
                            <div>
                                <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("listen_addr_label") }}</label>
                                <input v-model="state.settings.listen_addr" type="text" class="w-full bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-600 rounded-lg p-2.5 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 outline-none">
                                <p class="text-xs text-gray-500 dark:text-slate-500 mt-1">{{ t("listen_addr_hint") }}</p>
                            </div>
                        </div>
                    </div>

                    <div>
                        <h3 class="text-lg font-medium text-gray-900 dark:text-white mb-4 border-b border-gray-300 dark:border-slate-700 pb-2">{{ t("cb_rules") }}</h3>
                        <div class="grid grid-cols-2 gap-4">
                            <div>
                                <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("cb_threshold_label") }}</label>
                                <input v-model.number="state.settings.breaker.failure_threshold" type="number" min="0" class="w-full bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-600 rounded-lg p-2.5 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 outline-none">
                            </div>
                            <div>
                                <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("cb_failure_window") }}</label>
                                <input v-model.number="state.settings.breaker.failure_window_seconds" type="number" min="0" class="w-full bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-600 rounded-lg p-2.5 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 outline-none">
                            </div>
                            <div>
                                <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("cb_initial_cooldown") }}</label>
                                <input v-model.number="state.settings.breaker.initial_cooldown_seconds" type="number" min="0" class="w-full bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-600 rounded-lg p-2.5 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 outline-none">
                            </div>
                            <div>
                                <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("cb_max_cooldown") }}</label>
                                <input v-model.number="state.settings.breaker.max_cooldown_seconds" type="number" min="0" class="w-full bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-600 rounded-lg p-2.5 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 outline-none">
                            </div>
                        </div>
                    </div>

                    <div class="pt-4 flex justify-end gap-3">
                        <button @click="resetSettings" class="bg-gray-200 dark:bg-slate-700 hover:bg-gray-300 dark:hover:bg-slate-600 text-gray-700 dark:text-white px-6 py-2 rounded-lg font-medium transition shadow-lg shadow-slate-500/20">
                            {{ t("btn_reset_default") }}
                        </button>
                        <button @click="saveSettings" class="bg-blue-600 hover:bg-blue-700 text-white px-6 py-2 rounded-lg font-medium transition shadow-lg shadow-blue-500/20">
                            {{ t("btn_save_settings") }}
                        </button>
                    </div>

                    <div class="border-t border-gray-300 dark:border-slate-700 pt-6 mt-6">
                        <h3 class="text-lg font-medium text-gray-900 dark:text-white mb-4">{{ t("section_google_oauth") }}</h3>
                        <p class="text-xs text-gray-500 dark:text-slate-400 mb-4">
                            {{ t("oauth_hint_1") }}<a href="https://console.cloud.google.com/apis/credentials" target="_blank" class="text-blue-400 hover:underline">{{ t("oauth_hint_link") }}</a> {{ t("oauth_hint_2") }}
                            {{ t("oauth_hint_3") }}
                            <code class="bg-white dark:bg-slate-800 px-1.5 py-0.5 rounded text-emerald-400 text-xs">{{ redirectUri }}</code>
                        </p>
                        <div class="space-y-4">
                            <div>
                                <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("oauth_client_id") }}</label>
                                <input v-model="state.settings.google_oauth_client_id" type="text" :placeholder="t('placeholder_gcloud_id')" class="w-full bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-600 rounded-lg p-2.5 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 outline-none text-sm font-mono">
                            </div>
                            <div>
                                <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("oauth_client_secret") }}</label>
                                <input v-model="state.settings.google_oauth_client_secret" type="password" :placeholder="t('placeholder_gcloud_secret')" class="w-full bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-600 rounded-lg p-2.5 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 outline-none text-sm font-mono">
                            </div>
                        </div>
                    </div>
                </div>
            </div>
    `
};