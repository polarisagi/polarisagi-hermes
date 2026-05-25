export default {
    name: 'clientsComponent',
    setup() {
        return {
            statuses: [],
            loading: false,

            async fetchStatuses() {
                if (Alpine.store('global').currentTab !== 'clients') return;
                this.loading = true;
                try {
                    const res = await fetch('/api/admin/clients/status');
                    if (res.ok) {
                        const data = await res.json();
                        this.statuses = data.clients || [];
                    }
                } catch (e) {
                    console.error(e);
                } finally {
                    this.loading = false;
                }
            },

            async applyConfig(clientName) {
                const gStore = Alpine.store('global');
                if(!confirm(gStore.lang === 'zh' ? \`确定要为 \${clientName} 应用代理配置吗？这将会备份并修改原有的配置。\` : \`Are you sure you want to apply proxy config for \${clientName}? This will backup and modify the original config.\`)) return;
                try {
                    const res = await fetch('/api/admin/clients/apply', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ client: clientName })
                    });
                    if (res.ok) {
                        gStore.showToast(gStore.t('client_apply_success'));
                        this.fetchStatuses();
                    } else {
                        const errText = await res.text();
                        gStore.showToast(gStore.t('client_apply_failed') + ': ' + errText, 'error');
                    }
                } catch (e) {
                    gStore.showToast(gStore.t('network_error'), 'error');
                }
            },

            async restoreConfig(clientName) {
                const gStore = Alpine.store('global');
                if(!confirm(gStore.lang === 'zh' ? \`确定要恢复 \${clientName} 的代理配置吗？\` : \`Are you sure you want to restore config for \${clientName}?\`)) return;
                try {
                    const res = await fetch('/api/admin/clients/restore', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ client: clientName })
                    });
                    if (res.ok) {
                        gStore.showToast(gStore.t('client_restore_success'));
                        this.fetchStatuses();
                    } else {
                        const errText = await res.text();
                        gStore.showToast(gStore.t('client_restore_failed') + ': ' + errText, 'error');
                    }
                } catch (e) {
                    gStore.showToast(gStore.t('network_error'), 'error');
                }
            },

            init() {
                this.fetchStatuses();
                
                this.$watch('$store.global.currentTab', (newTab) => {
                    if (newTab === 'clients') {
                        this.fetchStatuses();
                    }
                });
            }
        };
    },
    template: \`
        <div x-show="$store.global.currentTab === 'clients'" class="max-w-5xl mx-auto w-full">
            <div class="flex justify-between items-center mb-6">
                <h2 class="text-3xl font-bold" x-text="$store.global.t('tab_clients_title')"></h2>
                <button @click="fetchStatuses" class="btn btn-outline shadow-sm">
                    <span :class="{'animate-spin': loading}">🔄</span> <span x-text="$store.global.t('btn_refresh')"></span>
                </button>
            </div>
            
            <div class="card bg-base-100 shadow overflow-hidden">
                <div class="p-6 border-b border-base-300">
                    <h3 class="text-lg font-medium mb-2" x-text="$store.global.t('clients_setup_desc_title')"></h3>
                    <p class="text-sm text-base-content/60" x-text="$store.global.t('clients_setup_desc')"></p>
                </div>
                
                <div class="overflow-x-auto">
                    <table class="table table-zebra w-full">
                        <thead>
                            <tr>
                                <th x-text="$store.global.t('client_name')"></th>
                                <th x-text="$store.global.t('client_status')"></th>
                                <th x-text="$store.global.t('client_backup')"></th>
                                <th x-text="$store.global.t('client_error')"></th>
                                <th class="text-right" x-text="$store.global.t('actions')"></th>
                            </tr>
                        </thead>
                        <tbody>
                            <template x-if="statuses.length === 0">
                                <tr>
                                    <td colspan="5" class="text-center py-8 text-base-content/50" x-text="loading ? $store.global.t('loading') : $store.global.t('no_clients')"></td>
                                </tr>
                            </template>
                            <template x-for="client in statuses" :key="client.name">
                                <tr>
                                    <td>
                                        <div class="flex items-center gap-3">
                                            <div class="avatar placeholder">
                                                <div class="bg-neutral text-neutral-content rounded-full w-8">
                                                    <span x-text="client.name.charAt(0).toUpperCase()"></span>
                                                </div>
                                            </div>
                                            <div class="font-medium" x-text="client.name"></div>
                                        </div>
                                    </td>
                                    <td>
                                        <template x-if="client.is_configured">
                                            <span class="badge badge-success badge-sm" x-text="$store.global.t('client_configured')"></span>
                                        </template>
                                        <template x-if="!client.is_configured">
                                            <span class="badge badge-ghost badge-sm" x-text="$store.global.t('client_not_configured')"></span>
                                        </template>
                                    </td>
                                    <td>
                                        <template x-if="client.has_backup">
                                            <span class="text-success text-sm flex items-center gap-1">
                                                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path></svg>
                                                <span x-text="$store.global.t('client_has_backup')"></span>
                                            </span>
                                        </template>
                                        <template x-if="!client.has_backup">
                                            <span class="text-base-content/40 text-sm" x-text="$store.global.t('client_no_backup')"></span>
                                        </template>
                                    </td>
                                    <td class="text-sm text-error" x-text="client.error"></td>
                                    <td class="text-right">
                                        <div class="flex items-center justify-end gap-2">
                                            <button @click="applyConfig(client.name)" class="btn btn-sm btn-info btn-outline" x-text="$store.global.t('btn_apply_config')"></button>
                                            <button @click="restoreConfig(client.name)" :disabled="!client.has_backup" class="btn btn-sm btn-warning btn-outline" x-text="$store.global.t('btn_restore_config')"></button>
                                        </div>
                                    </td>
                                </tr>
                            </template>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    \`
};
