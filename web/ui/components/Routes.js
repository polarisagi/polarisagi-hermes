import { state, t, showToast, protocolLabel, protocolClass, protocolBadge } from '../store.js';

const { ref, computed, onMounted } = Vue;

export default {
    name: 'Routes',
    setup() {
        const routeModal = ref({ show: false, isEdit: false });
        const routeForm = ref({
            id: 0,
            source_protocol: 'openai',
            target_protocol: 'openai',
            model_mappings: [{ match: '', target: '' }],
            status: 1
        });
        const sourceModels = ref([]);
        const targetModels = ref([]);

        const VALID_ROUTES = {
            anthropic: [
                { value: 'anthropic', label: t('route_anthropic_direct') },
                { value: 'google',    label: t('route_anthropic_google') },
                { value: 'openai',    label: t('route_anthropic_openai') },
            ],
            openai: [
                { value: 'openai',  label: t('route_openai_direct') },
                { value: 'google',  label: t('route_openai_google') },
            ],
            google: [
                { value: 'google', label: t('route_google_direct') },
            ],
        };

        const availableTargetProtocols = computed(() =>
            VALID_ROUTES[routeForm.value.source_protocol] || []
        );

        const routeTypeDesc = computed(() => {
            const descs = {
                'anthropic_anthropic': t('desc_anthropic_direct'),
                'anthropic_google': t('desc_anthropic_google'),
                'anthropic_openai': t('desc_anthropic_openai'),
                'openai_openai': t('desc_openai_direct'),
                'openai_google': t('desc_openai_google'),
                'google_google': t('desc_google_direct'),
            };
            return descs[\`\${routeForm.value.source_protocol}_\${routeForm.value.target_protocol}\`] || '';
        });

        const fetchRoutes = async () => {
            if (state.currentTab !== 'routes') return;
            try {
                const res = await fetch('/api/admin/routes');
                const data = await res.json() || [];
                data.forEach(r => {
                    if (!Array.isArray(r.model_mappings)) r.model_mappings = [];
                });
                state.routes = data;
            } catch (e) { console.error(e) }
        };

        const fetchAllModels = async () => {
            try {
                const res = await fetch('/api/admin/models');
                const json = await res.json();
                state.allModels = json.models || [];
            } catch (e) { console.error(e) }
        };

        const getModelsForProtocol = (protocol) => {
            if (!protocol) return [];
            return state.allModels.filter(m => m.protocol === protocol);
        };

        const onSourceProtocolChange = () => {
            sourceModels.value = getModelsForProtocol(routeForm.value.source_protocol);
            const validTargets = VALID_ROUTES[routeForm.value.source_protocol] || [];
            if (validTargets.length > 0 && !validTargets.find(t => t.value === routeForm.value.target_protocol)) {
                routeForm.value.target_protocol = validTargets[0].value;
            }
            onTargetProtocolChange();
        };

        const onTargetProtocolChange = () => {
            targetModels.value = getModelsForProtocol(routeForm.value.target_protocol);
        };

        const openRouteModal = (route = null) => {
            if (route) {
                const mappings = Array.isArray(route.model_mappings) && route.model_mappings.length > 0
                    ? route.model_mappings.map(m => ({ match: m.match || '', target: m.target || '' }))
                    : [{ match: '', target: '' }];
                routeForm.value = {
                    id: route.id,
                    source_protocol: route.source_protocol || 'openai',
                    target_protocol: route.target_protocol || 'openai',
                    model_mappings: mappings,
                    status: route.status
                };
                routeModal.value = { show: true, isEdit: true };
            } else {
                routeForm.value = {
                    id: 0,
                    source_protocol: 'openai',
                    target_protocol: 'openai',
                    model_mappings: [{ match: '', target: '' }],
                    status: 1
                };
                routeModal.value = { show: true, isEdit: false };
            }
            onSourceProtocolChange();
            onTargetProtocolChange();
        };

        const addMapping = () => {
            routeForm.value.model_mappings.push({ match: '', target: '' });
        };

        const removeMapping = (index) => {
            if (routeForm.value.model_mappings.length > 1) {
                routeForm.value.model_mappings.splice(index, 1);
            }
        };

        const saveRoute = async () => {
            const validMappings = routeForm.value.model_mappings.filter(m => m.match.trim() !== '');
            if (validMappings.length === 0) {
                showToast(t('err_empty_mapping'), 'error');
                return;
            }
            if (!routeForm.value.source_protocol || !routeForm.value.target_protocol) {
                showToast(t('err_empty_protocols'), 'error');
                return;
            }

            try {
                const method = routeModal.value.isEdit ? 'PUT' : 'POST';
                const payload = {
                    id: routeForm.value.id,
                    source_protocol: routeForm.value.source_protocol,
                    target_protocol: routeForm.value.target_protocol,
                    model_mappings: validMappings,
                    status: routeForm.value.status
                };
                const res = await fetch('/api/admin/routes', {
                    method,
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                if (res.ok) {
                    showToast(routeModal.value.isEdit ? t('route_updated') : t('route_added'));
                    routeModal.value.show = false;
                    fetchRoutes();
                } else {
                    const err = await res.text();
                    showToast(t('save_failed') + ': ' + err, 'error');
                }
            } catch(e) {
                showToast(t('network_error'), 'error');
            }
        };

        const deleteRoute = async (id) => {
            if(!confirm(state.lang === 'zh' ? '确定要删除这个路由吗？' : 'Are you sure you want to delete this route?')) return;
            try {
                const res = await fetch(\`/api/admin/routes?id=\${id}\`, { method: 'DELETE' });
                if (res.ok) {
                    showToast(t('route_deleted'));
                    fetchRoutes();
                } else {
                    showToast(t('delete_failed'), 'error');
                }
            } catch(e) {
                showToast(t('network_error'), 'error');
            }
        };

        onMounted(() => {
            fetchRoutes();
            fetchAllModels();
        });

        Vue.watch(() => state.currentTab, (newTab) => {
            if (newTab === 'routes') {
                fetchRoutes();
                fetchAllModels();
            }
        });

        return {
            state, t, protocolLabel, protocolClass, protocolBadge,
            routeModal, routeForm, openRouteModal, saveRoute, deleteRoute,
            addMapping, removeMapping, availableTargetProtocols, routeTypeDesc,
            sourceModels, targetModels, onSourceProtocolChange, onTargetProtocolChange
        };
    },
    template: \`
            <div v-show="state.currentTab === 'routes'" class="max-w-6xl mx-auto">
                <div class="flex justify-between items-center mb-6">
                    <div>
                        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t("tab_routes_title") }}</h2>
                        <p class="text-gray-500 dark:text-slate-400 text-sm mt-1">{{ t("routes_subtitle") }}</p>
                    </div>
                    <button @click="openRouteModal()" class="bg-pink-600 hover:bg-pink-700 text-white px-4 py-2 rounded-lg transition font-medium text-sm flex items-center gap-2 shadow-lg shadow-pink-500/20">
                        <span>+</span> {{ t("btn_add_new_route") }}
                    </button>
                </div>
                
                <div class="card rounded-xl shadow-lg overflow-hidden">
                    <table class="w-full text-sm text-left text-gray-700 dark:text-slate-300">
                        <thead class="text-xs text-gray-500 dark:text-slate-400 uppercase bg-white dark:bg-slate-800 border-b border-gray-300 dark:border-slate-700">
                            <tr>
                                <th class="px-6 py-4">{{ t("route_header_source") }}</th>
                                <th class="px-6 py-4">{{ t("route_header_target") }}</th>
                                <th class="px-6 py-4">{{ t("route_header_mapping") }}</th>
                                <th class="px-6 py-4 text-center">{{ t("table_status") }}</th>
                                <th class="px-6 py-4 text-right">{{ t("actions") }}</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr v-for="route in state.routes" :key="route.id" class="border-b border-gray-300 dark:border-slate-700 hover:bg-gray-200 dark:bg-slate-700/50">
                                <td class="px-6 py-4">
                                    <span :class="protocolBadge(route.source_protocol)" class="text-[10px] px-2 py-0.5 rounded-full font-bold uppercase border">{{ protocolLabel(route.source_protocol) }}</span>
                                </td>
                                <td class="px-6 py-4">
                                    <div :class="'text-sm font-medium ' + protocolClass(route.target_protocol)">{{ protocolLabel(route.target_protocol) }}</div>
                                    <div class="text-xs text-gray-500 dark:text-slate-500 mt-0.5">{{ {
                                        'anthropic_anthropic': t('route_direct'),
                                        'anthropic_google':    'Anthropic→Gemini/GEAP',
                                        'anthropic_openai':    'Anthropic→OpenAI',
                                        'openai_openai':       t('route_direct'),
                                        'openai_google':       'OpenAI→Vertex',
                                        'google_google':       t('route_direct'),
                                    }[route.source_protocol + '_' + route.target_protocol] || '' }}</div>
                                </td>
                                <td class="px-6 py-4">
                                    <div class="flex flex-wrap gap-1.5">
                                        <span v-for="(m, i) in (route.model_mappings || [])" :key="i"
                                            class="bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-700 rounded px-2 py-0.5 text-xs">
                                            <span class="text-blue-400 font-mono">{{ m.match }}</span>
                                            <span class="text-gray-500 dark:text-slate-500 mx-1">→</span>
                                            <span class="text-emerald-400 font-mono">{{ m.target }}</span>
                                        </span>
                                        <span v-if="!route.model_mappings || route.model_mappings.length === 0" class="text-gray-500 dark:text-slate-500 text-xs">{{ t("no_mapping") }}</span>
                                    </div>
                                </td>
                                <td class="px-6 py-4 text-center">
                                    <span v-if="route.status === 1" class="text-emerald-400 text-xs bg-emerald-400/10 px-2 py-1 rounded">{{ t("status_enabled_short") }}</span>
                                    <span v-else class="text-gray-500 dark:text-slate-500 text-xs bg-gray-200 dark:bg-slate-500/10 px-2 py-1 rounded">{{ t("status_disabled_short") }}</span>
                                </td>
                                <td class="px-6 py-4 text-right space-x-2">
                                    <button @click="openRouteModal(route)" class="text-blue-400 hover:text-blue-300 text-xs">{{ t("edit") }}</button>
                                    <button @click="deleteRoute(route.id)" class="text-red-400 hover:text-red-300 text-xs">{{ t("delete") }}</button>
                                </td>
                            </tr>
                            <tr v-if="state.routes.length === 0">
                                <td colspan="5" class="px-6 py-8 text-center text-gray-500 dark:text-slate-500">{{ t("no_routes") }}</td>
                            </tr>
                        </tbody>
                    </table>
                </div>

            <!-- Route Editor Modal -->
            <div v-if="routeModal.show" class="fixed inset-0 bg-gray-500/50 dark:bg-slate-900/80 backdrop-blur-sm z-50 flex items-center justify-center p-4 text-left">
                <div class="bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-700 rounded-xl shadow-2xl w-full max-w-lg max-h-[90vh] flex flex-col">
                    <div class="p-6 border-b border-gray-300 dark:border-slate-700 flex justify-between items-center">
                        <h3 class="text-xl font-bold text-gray-900 dark:text-white">{{ routeModal.isEdit ? t("edit_route") : t("add_new_route") }}</h3>
                        <button @click="routeModal.show = false" class="text-gray-500 dark:text-slate-400 hover:text-white transition">✕</button>
                    </div>
                    <div class="p-6 overflow-y-auto flex-1 space-y-4">
                        <div class="grid grid-cols-2 gap-4">
                            <div>
                                <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("label_source_req") }}</label>
                                <select v-model="routeForm.source_protocol" @change="onSourceProtocolChange()" class="w-full bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded-lg p-2.5 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 outline-none">
                                    <option value="anthropic">Anthropic — Messages API</option>
                                    <option value="openai">OpenAI — Chat Completions API</option>
                                    <option value="google">{{ t("option_google_geap") }}</option>
                                </select>
                                <p class="text-xs text-gray-500 dark:text-slate-500 mt-1">{{ t("hint_client_protocol") }}</p>
                            </div>
                            <div>
                                <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("label_target_req") }}</label>
                                <select v-model="routeForm.target_protocol" @change="onTargetProtocolChange()" class="w-full bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded-lg p-2.5 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 outline-none">
                                    <option v-for="tp in availableTargetProtocols" :key="tp.value" :value="tp.value">{{ tp.label }}</option>
                                </select>
                                <p class="text-xs text-emerald-400/80 mt-1" v-if="routeTypeDesc">{{ routeTypeDesc }}</p>
                                <p class="text-xs text-gray-500 dark:text-slate-500 mt-1" v-else>{{ t("hint_upstream_protocol") }}</p>
                            </div>
                        </div>

                        <div class="border-t border-gray-300 dark:border-slate-700 pt-4">
                            <div class="flex justify-between items-center mb-3">
                                <label class="block text-sm font-medium text-gray-700 dark:text-slate-300">{{ t("label_mappings_req") }}</label>
                                <button @click="addMapping" class="text-blue-400 hover:text-blue-300 text-xs px-2 py-1 rounded border border-blue-500/30 hover:border-blue-400 transition">
                                    {{ t("btn_add_mapping_simple") }}
                                </button>
                            </div>
                            <p class="text-xs text-gray-500 dark:text-slate-500 mb-3">{{ t("hint_mapping_desc") }}</p>
                            
                            <div class="space-y-2">
                                <div v-for="(mapping, index) in routeForm.model_mappings" :key="index"
                                    class="flex items-center gap-2 bg-gray-50 dark:bg-slate-900/50 rounded-lg p-2 border border-gray-300 dark:border-slate-700">
                                    <span class="text-gray-500 dark:text-slate-500 text-xs w-5">{{ index + 1 }}.</span>
                                    <input v-model="mapping.match" type="text" :placeholder="t('placeholder_match_model')" 
                                        :list="'match-list-' + index"
                                        class="flex-1 bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded p-2 text-sm text-blue-400 font-mono outline-none focus:ring-1 focus:ring-blue-500">
                                    <datalist :id="'match-list-' + index">
                                        <option value="*">{{ t("option_all_models") }}</option>
                                        <option v-for="m in sourceModels" :key="m.name" :value="m.name">{{ m.display_name }}</option>
                                    </datalist>
                                    <span class="text-gray-400 dark:text-slate-600 text-sm">→</span>
                                    <input v-model="mapping.target" type="text" :placeholder="t('placeholder_target_model')" 
                                        :list="'target-list-' + index"
                                        class="flex-1 bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded p-2 text-sm text-emerald-400 font-mono outline-none focus:ring-1 focus:ring-emerald-500">
                                    <datalist :id="'target-list-' + index">
                                        <option v-for="m in targetModels" :key="m.name" :value="m.name">{{ m.display_name }}</option>
                                    </datalist>
                                    <button @click="removeMapping(index)" 
                                        class="text-red-400 hover:text-red-300 transition text-lg px-1"
                                        :disabled="routeForm.model_mappings.length <= 1">×</button>
                                </div>
                            </div>
                        </div>

                        <div>
                            <label class="block text-sm font-medium text-gray-700 dark:text-slate-300 mb-1">{{ t("label_route_status") }}</label>
                            <select v-model="routeForm.status" class="w-full bg-gray-50 dark:bg-slate-900 border border-gray-300 dark:border-slate-600 rounded-lg p-2.5 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 outline-none">
                                <option :value="1">{{ t("status_enabled_short") }}</option>
                                <option :value="0">{{ t("status_disabled_short") }}</option>
                            </select>
                        </div>
                    </div>
                    <div class="p-6 border-t border-gray-300 dark:border-slate-700 flex justify-end gap-3 bg-white dark:bg-slate-800/50">
                        <button @click="routeModal.show = false" class="px-5 py-2 rounded-lg text-gray-700 dark:text-slate-300 hover:bg-gray-200 dark:bg-slate-700 transition">{{ t("cancel") }}</button>
                        <button @click="saveRoute" class="bg-pink-600 hover:bg-pink-700 text-white px-6 py-2 rounded-lg font-medium transition shadow-lg shadow-pink-500/20">
                            {{ t("btn_save_simple") }}
                        </button>
                    </div>
                </div>
            </div>
        </div>
    \`
};