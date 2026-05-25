import { state, t, showToast } from '../store.js';

export default {
    name: 'Clients',
    setup() {
        const statuses = Vue.ref([]);
        const loading = Vue.ref(false);

        const fetchStatuses = async () => {
            loading.value = true;
            try {
                const res = await fetch('/api/admin/clients/status');
                if (res.ok) {
                    const data = await res.json();
                    statuses.value = data.clients || [];
                }
            } catch (e) {
                console.error(e);
            } finally {
                loading.value = false;
            }
        };

        const applyConfig = async (clientName) => {
            if(!confirm(state.lang === 'zh' ? `确定要为 ${clientName} 应用代理配置吗？这将会备份并修改原有的配置。` : `Are you sure you want to apply proxy config for ${clientName}? This will backup and modify the original config.`)) return;
            try {
                const res = await fetch('/api/admin/clients/apply', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ client: clientName })
                });
                if (res.ok) {
                    showToast(t('client_apply_success'));
                    fetchStatuses();
                } else {
                    const errText = await res.text();
                    showToast(t('client_apply_failed') + ': ' + errText, 'error');
                }
            } catch (e) {
                showToast(t('network_error'), 'error');
            }
        };

        const restoreConfig = async (clientName) => {
            if(!confirm(state.lang === 'zh' ? `确定要恢复 ${clientName} 的代理配置吗？` : `Are you sure you want to restore config for ${clientName}?`)) return;
            try {
                const res = await fetch('/api/admin/clients/restore', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ client: clientName })
                });
                if (res.ok) {
                    showToast(t('client_restore_success'));
                    fetchStatuses();
                } else {
                    const errText = await res.text();
                    showToast(t('client_restore_failed') + ': ' + errText, 'error');
                }
            } catch (e) {
                showToast(t('network_error'), 'error');
            }
        };

        Vue.onMounted(() => {
            fetchStatuses();
        });

        // 刷新定时器，当处于此 tab 时自动刷新
        Vue.watch(() => state.currentTab, (newTab) => {
            if (newTab === 'clients') {
                fetchStatuses();
            }
        });

        return {
            state,
            t,
            statuses,
            loading,
            applyConfig,
            restoreConfig,
            fetchStatuses
        };
    },
    template: `
        <div v-show="state.currentTab === 'clients'" class="max-w-5xl mx-auto">
            <div class="flex justify-between items-center mb-6">
                <h2 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t("tab_clients_title") }}</h2>
                <button @click="fetchStatuses" class="flex items-center gap-2 bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-600 hover:bg-gray-50 dark:hover:bg-slate-700 text-gray-700 dark:text-gray-300 px-4 py-2 rounded-lg font-medium transition shadow-sm">
                    <span :class="{'animate-spin': loading}">🔄</span> {{ t("btn_refresh") }}
                </button>
            </div>
            
            <div class="bg-white dark:bg-slate-800 rounded-xl shadow-sm border border-gray-200 dark:border-slate-700 overflow-hidden">
                <div class="p-6 border-b border-gray-200 dark:border-slate-700">
                    <h3 class="text-lg font-medium text-gray-900 dark:text-white mb-2">{{ t("clients_setup_desc_title") }}</h3>
                    <p class="text-sm text-gray-500 dark:text-slate-400">
                        {{ t("clients_setup_desc") }}
                    </p>
                </div>
                
                <div class="overflow-x-auto">
                    <table class="w-full text-left border-collapse">
                        <thead>
                            <tr class="bg-gray-50 dark:bg-slate-900/50 border-b border-gray-200 dark:border-slate-700 text-xs uppercase tracking-wider text-gray-500 dark:text-slate-400">
                                <th class="px-6 py-4 font-semibold">{{ t("client_name") }}</th>
                                <th class="px-6 py-4 font-semibold">{{ t("client_status") }}</th>
                                <th class="px-6 py-4 font-semibold">{{ t("client_backup") }}</th>
                                <th class="px-6 py-4 font-semibold">{{ t("client_error") }}</th>
                                <th class="px-6 py-4 font-semibold text-right">{{ t("actions") }}</th>
                            </tr>
                        </thead>
                        <tbody class="divide-y divide-gray-100 dark:divide-slate-700/50">
                            <tr v-if="statuses.length === 0" class="hover:bg-gray-50 dark:hover:bg-slate-800/50 transition">
                                <td colspan="5" class="px-6 py-8 text-center text-gray-500 dark:text-slate-400">
                                    {{ loading ? t('loading') : t('no_clients') }}
                                </td>
                            </tr>
                            <tr v-for="client in statuses" :key="client.name" class="hover:bg-gray-50 dark:hover:bg-slate-800/50 transition">
                                <td class="px-6 py-4">
                                    <div class="flex items-center gap-3">
                                        <div class="w-8 h-8 rounded-full bg-blue-100 dark:bg-blue-900/30 flex items-center justify-center text-blue-600 dark:text-blue-400 font-bold">
                                            {{ client.name.charAt(0).toUpperCase() }}
                                        </div>
                                        <div class="font-medium text-gray-900 dark:text-white">{{ client.name }}</div>
                                    </div>
                                </td>
                                <td class="px-6 py-4">
                                    <span v-if="client.is_configured" class="inline-flex items-center px-2.5 py-1 rounded-full text-xs font-medium bg-emerald-100 dark:bg-emerald-900/30 text-emerald-800 dark:text-emerald-400">
                                        {{ t('client_configured') }}
                                    </span>
                                    <span v-else class="inline-flex items-center px-2.5 py-1 rounded-full text-xs font-medium bg-gray-100 dark:bg-slate-700 text-gray-800 dark:text-gray-300">
                                        {{ t('client_not_configured') }}
                                    </span>
                                </td>
                                <td class="px-6 py-4">
                                    <span v-if="client.has_backup" class="text-emerald-500 dark:text-emerald-400 text-sm flex items-center gap-1">
                                        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path></svg>
                                        {{ t('client_has_backup') }}
                                    </span>
                                    <span v-else class="text-gray-400 dark:text-slate-500 text-sm">
                                        {{ t('client_no_backup') }}
                                    </span>
                                </td>
                                <td class="px-6 py-4 text-sm text-red-500 dark:text-red-400">
                                    {{ client.error }}
                                </td>
                                <td class="px-6 py-4 text-right">
                                    <div class="flex items-center justify-end gap-2">
                                        <button @click="applyConfig(client.name)" class="px-3 py-1.5 bg-blue-50 text-blue-600 hover:bg-blue-100 dark:bg-blue-900/30 dark:text-blue-400 dark:hover:bg-blue-900/50 rounded text-sm font-medium transition">
                                            {{ t('btn_apply_config') }}
                                        </button>
                                        <button @click="restoreConfig(client.name)" :disabled="!client.has_backup" :class="client.has_backup ? 'bg-amber-50 text-amber-600 hover:bg-amber-100 dark:bg-amber-900/30 dark:text-amber-400 dark:hover:bg-amber-900/50' : 'bg-gray-50 text-gray-400 dark:bg-slate-800 dark:text-slate-500 cursor-not-allowed'" class="px-3 py-1.5 rounded text-sm font-medium transition">
                                            {{ t('btn_restore_config') }}
                                        </button>
                                    </div>
                                </td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    `
};
