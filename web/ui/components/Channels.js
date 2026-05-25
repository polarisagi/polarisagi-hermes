import { state, t, showToast, formatNum, formatShortDate, protocolLabel, protocolBadge } from '../store.js';

const { ref, onMounted } = Vue;

export default {
    name: 'Channels',
    setup() {
        const nodeModal = ref({ show: false, isEdit: false });
        
        const toDatetimeLocal = (dt) => {
            if (!dt) return '';
            dt = dt.trim();
            dt = dt.replace(/Z$/, '').replace(/[+-]\\d{2}:\\d{2}$/, '');
            if (dt.length === 10) return dt + 'T00:00:00';
            return dt.replace(' ', 'T');
        };
        const fromDatetimeLocal = (dt) => dt ? dt.trim().replace('T', ' ') : '';
        const todayPrefix = () => {
            const d = new Date();
            const pad = n => String(n).padStart(2, '0');
            return `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}`;
        };

        const nodeForm = ref({
            id: 0, provider: 'openai', name: '', credentials: '', project_id: '', location: 'global', base_url: '',
            priority: 10, limit_percent: 90.0, balance: 0.0, min_request_interval_sec: 0, concurrency: 0,
            valid_from: `${todayPrefix()}T00:00:00`, valid_to: `2099-12-31T23:59:59`, status: 1
        });

        const usagePercent = (node) => {
            if (!node.balance || node.balance <= 0) return 0;
            return ((node.used_amount || 0) / node.balance) * 100;
        };

        const fetchNodes = async () => {
            if (state.currentTab !== 'nodes' && state.currentTab !== 'routes') return;
            try {
                const res = await fetch('/api/admin/nodes');
                state.nodes = await res.json() || [];
            } catch (e) { console.error(e) }
        };

        const openNodeModal = (node = null) => {
            if (node) {
                nodeForm.value = {
                    ...node,
                    credentials: '',
                    valid_from: toDatetimeLocal(node.valid_from),
                    valid_to: toDatetimeLocal(node.valid_to),
                };
                nodeModal.value = { show: true, isEdit: true };
            } else {
                const today = todayPrefix();
                nodeForm.value = {
                    id: 0, provider: 'openai', name: '', credentials: '', project_id: '', location: 'global', base_url: '',
                    priority: 10, limit_percent: 90.0, balance: 0.0, min_request_interval_sec: 0, concurrency: 0,
                    valid_from: `${today}T00:00:00`, valid_to: `2099-12-31T23:59:59`, status: 1
                };
                nodeModal.value = { show: true, isEdit: false };
            }
        };

        const saveNode = async () => {
            if (!nodeForm.value.name || (!nodeModal.value.isEdit && !nodeForm.value.credentials)) {
                showToast(t('err_empty_node'), 'error');
                return;
            }
            if (nodeForm.value.provider === 'google' && !nodeForm.value.project_id) {
                showToast(t('err_gcp_project'), 'error');
                return;
            }
            if (nodeForm.value.priority < 0 || nodeForm.value.balance < 0 || nodeForm.value.limit_percent < 0) {
                showToast(t('err_negative_numbers'), 'error');
                return;
            }
            if (nodeForm.value.limit_percent > 100) {
                showToast(t('err_limit_exceed'), 'error');
                return;
            }
            if (nodeForm.value.concurrency < 0 || nodeForm.value.concurrency > 1000) {
                showToast('并发限制必须在 0 到 1000 之间', 'error');
                return;
            }

            try {
                const method = nodeModal.value.isEdit ? 'PUT' : 'POST';
                const payload = {
                    ...nodeForm.value,
                    valid_from: fromDatetimeLocal(nodeForm.value.valid_from),
                    valid_to: fromDatetimeLocal(nodeForm.value.valid_to),
                };
                const res = await fetch('/api/admin/nodes', {
                    method,
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                if (res.ok) {
                    showToast(nodeModal.value.isEdit ? t('node_updated') : t('node_added'));
                    nodeModal.value.show = false;
                    fetchNodes();
                } else {
                    const err = await res.text();
                    showToast(t('save_failed') + ': ' + err, 'error');
                }
            } catch(e) {
                showToast(t('network_error'), 'error');
            }
        };

        const deleteNode = async (id) => {
            if(!confirm(state.lang === 'zh' ? '确定要删除这个节点吗？此操作不可恢复。' : 'Are you sure you want to delete this node? This action cannot be undone.')) return;
            try {
                const res = await fetch(`/api/admin/nodes?id=${id}`, { method: 'DELETE' });
                if (res.ok) {
                    showToast(t('node_deleted'));
                    fetchNodes();
                } else {
                    showToast(t('delete_failed'), 'error');
                }
            } catch(e) {
                showToast(t('network_error'), 'error');
            }
        };

        const startGoogleAuth = () => {
            const isLocal = window.location.hostname === '127.0.0.1' || window.location.hostname === 'localhost';
            if (!isLocal) {
                alert(t("oauth_alert"));
                return;
            }
            
            const receiveMessage = (event) => {
                if (event.data && event.data.type === 'google_adc_auth' && event.data.data) {
                    nodeForm.value.credentials = event.data.data;
                    showToast(t('adc_filled'));
                    window.removeEventListener('message', receiveMessage);
                }
            };
            window.addEventListener('message', receiveMessage, false);

            const width = 600;
            const height = 700;
            const left = Math.max(0, (window.innerWidth - width) / 2 + window.screenX);
            const top = Math.max(0, (window.innerHeight - height) / 2 + window.screenY);
            window.open('/api/admin/oauth/google/start', 'GoogleAuth', `width=${width},height=${height},top=${top},left=${left}`);
        };

        onMounted(() => {
            fetchNodes();
        });

        // Watch for tab changes (handled centrally in app.js, or can be here)
        Vue.watch(() => state.currentTab, (newTab) => {
            if (newTab === 'nodes') fetchNodes();
        });

        Vue.watch(() => nodeForm.value.provider, (newVal) => {
            if (!nodeModal.value.isEdit && newVal === 'google') {
                nodeForm.value.concurrency = 1;
            } else if (!nodeModal.value.isEdit && newVal !== 'google') {
                nodeForm.value.concurrency = 0;
            }
        });

        return {
            state, t, formatNum, formatShortDate, protocolLabel, protocolBadge,
            nodeModal, nodeForm, openNodeModal, saveNode, deleteNode, startGoogleAuth,
            usagePercent
        };
    },
    template: `
            <div v-show="state.currentTab === 'channels'" class="max-w-6xl mx-auto">
                <div class="flex justify-between items-center mb-6">
                    <div>
                        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t("tab_nodes_title") }}</h2>
                        <p class="text-gray-500 dark:text-slate-400 text-sm mt-1">{{ t("channels_subtitle") }}</p>
                    </div>
                    <button @click="openNodeModal()" class="bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded-lg transition font-medium text-sm flex items-center gap-2 shadow-lg shadow-blue-500/20">
                        <span>+</span> {{ t("btn_add_new_node") }}
                    </button>
                </div>
                
                <div class="card rounded-xl shadow-lg overflow-hidden">
                    <table class="w-full text-sm text-left text-gray-700 dark:text-slate-300">
                        <thead class="text-xs text-gray-500 dark:text-slate-400 uppercase bg-white dark:bg-slate-800 border-b border-gray-300 dark:border-slate-700">
                            <tr>
                                <th class="px-4 py-3">{{ t("table_platform") }}</th>
                                <th class="px-4 py-3">{{ t("node_name") }}</th>
                                <th class="px-4 py-3 text-center">Pri / Concurrency</th>
                                <th class="px-4 py-3">{{ t("table_limit_usage") }}</th>
                                <th class="px-4 py-3">{{ t("valid_range") }}</th>
                                <th class="px-4 py-3 text-center w-20">{{ t("table_status") }}</th>
                                <th class="px-4 py-3 text-right w-24">{{ t("actions") }}</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr v-for="node in state.nodes" :key="node.id" class="border-b border-gray-300 dark:border-slate-700 hover:bg-gray-200 dark:bg-slate-700/50">
                                <td class="px-4 py-3">
                                    <span :class="protocolBadge(node.provider)"
                                        class="text-[10px] px-2 py-0.5 rounded-full font-bold uppercase border">{{ protocolLabel(node.provider) }}</span>
                                </td>
                                <td class="px-4 py-3 font-medium text-gray-900 dark:text-white">{{ node.name }}</td>
                                <td class="px-4 py-3 text-center text-xs">
                                    <div class="font-bold">Pri: {{ node.priority }}</div>
                                    <div class="text-gray-500">Con: {{ node.concurrency === 0 ? '∞' : node.concurrency }}</div>
                                </td>
                                <td class="px-4 py-3">
                                    <div v-if="node.balance > 0" class="space-y-1">
                                        <div class="flex items-center justify-between text-[10px]">
                                            <span class="text-gray-500 dark:text-slate-400">\${{ formatNum(node.used_amount || 0) }} / \${{ formatNum(node.balance) }}</span>
                                            <span :class="usagePercent(node) >= node.limit_percent ? 'text-red-400' : 'text-gray-500 dark:text-slate-500'">{{ usagePercent(node).toFixed(1) }}%</span>
                                        </div>
                                        <div class="w-full bg-gray-200 dark:bg-slate-700 rounded-full h-1.5">
                                            <div :class="usagePercent(node) >= node.limit_percent ? 'bg-red-500' : 'bg-emerald-500'" class="h-1.5 rounded-full transition-all" :style="{ width: Math.min(usagePercent(node), 100) + '%' }"></div>
                                        </div>
                                    </div>
                                    <span v-else class="text-gray-500 dark:text-slate-500 text-xs">{{ t("no_limit_text") }}</span>
                                </td>
                                <td class="px-4 py-3 text-xs text-gray-500 dark:text-slate-400">
                                    <span v-if="node.valid_from && node.valid_to">
                                        {{ formatShortDate(node.valid_from) }}<br><span class="text-gray-400 dark:text-slate-600">~</span> {{ formatShortDate(node.valid_to) }}
                                    </span>
                                    <span v-else class="text-gray-400 dark:text-slate-600">-</span>
                                </td>
                                <td class="px-4 py-3 text-center">
                                    <span v-if="node.status === 1" class="text-emerald-400 text-xs bg-emerald-400/10 px-2 py-1 rounded">{{ t("status_enabled_short") }}</span>
                                    <span v-else-if="node.status === 0" class="text-gray-500 dark:text-slate-500 text-xs bg-gray-200 dark:bg-slate-500/10 px-2 py-1 rounded">{{ t("status_disabled_short") }}</span>
                                    <span v-else-if="node.status === -1" class="text-red-500 text-xs bg-red-500/10 px-2 py-1 rounded">{{ t("status_exhausted_short") }}</span>
                                </td>
                                <td class="px-4 py-3 text-right space-x-2">
                                    <button @click="openNodeModal(node)" class="text-blue-400 hover:text-blue-300 text-xs">{{ t("edit") }}</button>
                                    <button @click="deleteNode(node.id)" class="text-red-400 hover:text-red-300 text-xs">{{ t("delete") }}</button>
                                </td>
                            </tr>
                            <tr v-if="state.nodes.length === 0">
                                <td colspan="7" class="px-6 py-8 text-center text-gray-500 dark:text-slate-500">{{ t("no_nodes") }}</td>
                            </tr>
                        </tbody>
                    </table>
                </div>

            <!-- Node Editor Modal -->
            <div v-if="nodeModal.show" class="fixed inset-0 bg-gray-500/50 dark:bg-slate-900/80 backdrop-blur-sm z-50 flex items-center justify-center p-4 text-left">
                <div class="bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-700 rounded-xl shadow-2xl w-full max-w-2xl max-h-[90vh] flex flex-col">
                    <div class="p-6 border-b border-gray-300 dark:border-slate-700 flex justify-between items-center">
                        <h3 class="text-xl font-bold text-gray-900 dark:text-white">{{ nodeModal.isEdit ? t("edit_node") : t("add_new_node") }}</h3>
                        <button @click="nodeModal.show = false" class="text-gray-500 dark:text-slate-400 hover:text-white transition">✕</button>
                    </div>
                    <div class="p-6 overflow-y-auto flex-1 space-y-5">
                        <!-- 区块 1: 基本信息 -->
                        <div class="p-4 bg-gray-50 dark:bg-slate-900/40 rounded-lg border border-gray-300 dark:border-slate-700/60 space-y-4">
                            <h4 class="text-xs font-semibold text-gray-500 dark:text-slate-400 uppercase tracking-wider">{{ t("section_basic") }}</h4>
                            <div class="grid grid-cols-2 gap-4">
                                <div>
                                    <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("protocol_type_req") }} <span class="text-red-400">*</span></label>
                                    <select v-model="nodeForm.provider" class="w-full bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded-lg p-2.5 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 outline-none">
                                        <option value="openai">OpenAI</option>
                                        <option value="google">Google Agent Platform</option>
                                        <option value="anthropic">Anthropic</option>
                                    </select>
                                </div>
                                <div>
                                    <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("node_name_req") }} <span class="text-red-400">*</span></label>
                                    <input v-model="nodeForm.name" type="text" :placeholder="t('placeholder_node_name')" class="w-full bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded-lg p-2.5 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 outline-none">
                                </div>
                            </div>
                            <div>
                                <div class="flex justify-between items-center mb-1">
                                    <label class="block text-sm font-medium text-gray-700 dark:text-slate-300">
                                        API Key / ADC JSON <span class="text-red-400">*</span>
                                        <span v-if="nodeForm.provider === 'google'" class="text-gray-500 dark:text-slate-500 font-normal text-xs ml-1">{{ t("hint_adc_paste") }}</span>
                                        <span v-else-if="nodeForm.provider === 'anthropic'" class="text-gray-500 dark:text-slate-500 font-normal text-xs ml-1">{{ t("hint_anthropic_format") }}</span>
                                        <span v-else class="text-gray-500 dark:text-slate-500 font-normal text-xs ml-1">{{ t("hint_sk_bearer") }}</span>
                                    </label>
                                    <button v-if="nodeForm.provider === 'google'" @click="startGoogleAuth" :title="t('tooltip_google_oauth')" class="text-xs bg-blue-600/20 text-blue-400 border border-blue-500/50 hover:bg-blue-600 hover:text-white px-2 py-1 rounded transition flex items-center gap-1">
                                        <span>🔑</span>{{ t("btn_oauth_auto") }}
                                    </button>
                                </div>
                                <textarea v-if="nodeForm.provider === 'google'" v-model="nodeForm.credentials" rows="3"
                                    :placeholder="nodeModal.isEdit ? t('placeholder_adc_edit') : t('placeholder_adc_new')"
                                    class="w-full bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded-lg p-2.5 text-gray-900 dark:text-white text-xs font-mono focus:ring-2 focus:ring-blue-500 outline-none resize-y"></textarea>
                                <input v-else v-model="nodeForm.credentials" type="password"
                                    :placeholder="nodeModal.isEdit ? t('placeholder_key_edit') : t('placeholder_key_new')"
                                    class="w-full bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded-lg p-2.5 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 outline-none">
                            </div>
                            <div class="grid grid-cols-2 gap-4" v-if="state.proMode">
                                <div>
                                    <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("priority") }}</label>
                                    <input v-model.number="nodeForm.priority" type="number" min="0" class="w-full bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded-lg p-2.5 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 outline-none">
                                    <p class="text-xs text-gray-500 dark:text-slate-500 mt-1">{{ t("priority_hint") }}</p>
                                </div>
                                <div>
                                    <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("min_interval_label") }}</label>
                                    <input v-model.number="nodeForm.min_request_interval_sec" type="number" min="0" class="w-full bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded-lg p-2.5 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 outline-none">
                                    <p class="text-xs text-gray-500 dark:text-slate-500 mt-1">{{ t("min_interval_hint") }}</p>
                                </div>
                            </div>
                            <div class="grid grid-cols-2 gap-4" v-if="state.proMode">
                                <div>
                                    <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("status") }}</label>
                                    <select v-model="nodeForm.status" class="w-full bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded-lg p-2.5 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 outline-none">
                                        <option :value="1">{{ t("status_option_enable") }}</option>
                                        <option :value="0">{{ t("status_option_disable") }}</option>
                                        <option :value="-1">{{ t("status_option_exhaust") }}</option>
                                    </select>
                                </div>
                                <div>
                                    <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">Concurrency (并发限制)</label>
                                    <input v-model.number="nodeForm.concurrency" type="number" min="0" max="1000" class="w-full bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded-lg p-2.5 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 outline-none">
                                    <p class="text-xs text-gray-500 dark:text-slate-500 mt-1">0 为无限制，上限 1000</p>
                                </div>
                            </div>
                        </div>

                        <!-- 区块 2: 供应商配置 -->
                        <div class="p-4 bg-gray-50 dark:bg-slate-900/40 rounded-lg border border-gray-300 dark:border-slate-700/60 space-y-4">
                            <h4 class="text-xs font-semibold text-gray-500 dark:text-slate-400 uppercase tracking-wider">{{ t("section_provider") }}</h4>
                            <div v-if="nodeForm.provider === 'google'" class="grid grid-cols-2 gap-4">
                                <div>
                                    <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("gcp_project_id") }} <span class="text-red-400">*</span></label>
                                    <input v-model="nodeForm.project_id" type="text" placeholder="your-gcp-project-id" class="w-full bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded-lg p-2 text-gray-900 dark:text-white outline-none focus:ring-1 focus:ring-blue-500">
                                </div>
                                <div>
                                    <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("gcp_location") }}</label>
                                    <input v-model="nodeForm.location" type="text" placeholder="global" class="w-full bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded-lg p-2 text-gray-900 dark:text-white outline-none focus:ring-1 focus:ring-blue-500">
                                    <p class="text-xs text-gray-500 dark:text-slate-500 mt-1">{{ t("hint_location") }}</p>
                                </div>
                            </div>
                            <div v-if="state.proMode">
                                <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("base_url_optional") }}</label>
                                <input v-model="nodeForm.base_url" type="text" :placeholder="t('placeholder_baseurl')" class="w-full bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded-lg p-2 text-gray-900 dark:text-white outline-none focus:ring-1 focus:ring-blue-500">
                                <p class="text-xs text-gray-500 dark:text-slate-500 mt-1">{{ t("hint_custom_endpoint") }}</p>
                            </div>
                        </div>

                        <!-- 区块 3: 计费与有效期 -->
                        <div v-if="state.proMode" class="p-4 bg-gray-50 dark:bg-slate-900/40 rounded-lg border border-gray-300 dark:border-slate-700/60 space-y-4">
                            <h4 class="text-xs font-semibold text-gray-500 dark:text-slate-400 uppercase tracking-wider">{{ t("section_billing_validity") }}</h4>
                            <div class="grid grid-cols-2 gap-4">
                                <div>
                                    <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("label_total_balance") }}</label>
                                    <input v-model.number="nodeForm.balance" type="number" min="0" step="0.01" placeholder="0.00" class="w-full bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded-lg p-2 text-gray-900 dark:text-white outline-none focus:ring-1 focus:ring-blue-500">
                                    <p class="text-xs text-gray-500 dark:text-slate-500 mt-1">{{ t("hint_unlimited") }}</p>
                                </div>
                                <div>
                                    <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("label_limit_percent") }}</label>
                                    <input v-model.number="nodeForm.limit_percent" type="number" min="0" max="100" step="0.1" class="w-full bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded-lg p-2 text-gray-900 dark:text-white outline-none focus:ring-1 focus:ring-blue-500">
                                    <p class="text-xs text-gray-500 dark:text-slate-500 mt-1">{{ t("hint_limit_percent") }}</p>
                                </div>
                                <div>
                                    <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("label_valid_from") }}</label>
                                    <input v-model="nodeForm.valid_from" type="datetime-local" step="1" style="color-scheme:dark"
                                        class="w-full bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded-lg px-3 py-2 text-gray-900 dark:text-white outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent transition">
                                </div>
                                <div>
                                    <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("label_valid_to") }}</label>
                                    <input v-model="nodeForm.valid_to" type="datetime-local" step="1" style="color-scheme:dark"
                                        class="w-full bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded-lg px-3 py-2 text-gray-900 dark:text-white outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent transition">
                                    <p class="text-xs text-gray-500 dark:text-slate-500 mt-1">{{ t("hint_expire_auto") }}</p>
                                </div>
                            </div>
                        </div>
                    </div>
                    <div class="p-6 border-t border-gray-300 dark:border-slate-700 flex justify-end gap-3 bg-white dark:bg-slate-800/50">
                        <button @click="nodeModal.show = false" class="px-5 py-2 rounded-lg text-gray-700 dark:text-slate-300 hover:bg-gray-200 dark:bg-slate-700 transition">{{ t("cancel") }}</button>
                        <button @click="saveNode" class="bg-blue-600 hover:bg-blue-700 text-white px-6 py-2 rounded-lg font-medium transition shadow-lg shadow-blue-500/20">
                            {{ t("btn_save_simple") }}
                        </button>
                    </div>
                </div>
            </div>
        </div>
    `
};