export default {
    name: 'rulesComponent',
    setup() {
        return {
            routeModal: { show: false, isEdit: false },
            routeForm: {
                id: 0,
                source_protocol: 'openai',
                target_protocol: 'openai',
                model_mappings: [{ match: '', target: '' }],
                status: 1
            },
            sourceModels: [],
            targetModels: [],

            get VALID_ROUTES() {
                const gStore = Alpine.store('global');
                return {
                    anthropic: [
                        { value: 'anthropic', label: gStore.t('route_anthropic_direct') },
                        { value: 'google',    label: gStore.t('route_anthropic_google') },
                        { value: 'openai',    label: gStore.t('route_anthropic_openai') },
                        { value: 'ollama',    label: gStore.t('route_anthropic_ollama') },
                    ],
                    openai: [
                        { value: 'openai',  label: gStore.t('route_openai_direct') },
                        { value: 'google',  label: gStore.t('route_openai_google') },
                        { value: 'ollama',  label: gStore.t('route_openai_ollama') },
                    ],
                    google: [
                        { value: 'google', label: gStore.t('route_google_direct') },
                    ],
                };
            },

            get availableTargetProtocols() {
                return this.VALID_ROUTES[this.routeForm.source_protocol] || [];
            },

            get routeTypeDesc() {
                const gStore = Alpine.store('global');
                    'anthropic_anthropic': gStore.t('desc_anthropic_direct'),
                    'anthropic_google': gStore.t('desc_anthropic_google'),
                    'anthropic_openai': gStore.t('desc_anthropic_openai'),
                    'anthropic_ollama': gStore.t('desc_anthropic_ollama'),
                    'openai_openai': gStore.t('desc_openai_direct'),
                    'openai_google': gStore.t('desc_openai_google'),
                    'openai_ollama': gStore.t('desc_openai_ollama'),
                    'google_google': gStore.t('desc_google_direct'),
                };
                return descs[`${this.routeForm.source_protocol}_${this.routeForm.target_protocol}`] || '';
            },

            getDescShort(source, target) {
                const gStore = Alpine.store('global');
                    'anthropic_anthropic': gStore.t('route_direct'),
                    'anthropic_google':    'Anthropic→Gemini/GEAP',
                    'anthropic_openai':    'Anthropic→OpenAI',
                    'anthropic_ollama':    'Anthropic→Ollama',
                    'openai_openai':       gStore.t('route_direct'),
                    'openai_google':       'OpenAI→Vertex',
                    'openai_ollama':       'OpenAI→Ollama',
                    'google_google':       gStore.t('route_direct'),
                };
                return descs[`${source}_${target}`] || '';
            },

            async fetchRoutes() {
                if (Alpine.store('global').currentTab !== 'rules') return;
                try {
                    const res = await fetch('/api/admin/routes');
                    const data = await res.json() || [];
                    data.forEach(r => {
                        if (!Array.isArray(r.model_mappings)) r.model_mappings = [];
                    });
                    Alpine.store('global').routes = data;
                } catch (e) { console.error(e) }
            },

            async fetchAllModels() {
                try {
                    const res = await fetch('/api/admin/models');
                    const json = await res.json();
                    Alpine.store('global').allModels = json.models || [];
                } catch (e) { console.error(e) }
            },

            getModelsForProtocol(protocol) {
                if (!protocol) return [];
                return Alpine.store('global').allModels.filter(m => m.protocol === protocol);
            },

            onSourceProtocolChange() {
                this.sourceModels = this.getModelsForProtocol(this.routeForm.source_protocol);
                const validTargets = this.VALID_ROUTES[this.routeForm.source_protocol] || [];
                if (validTargets.length > 0 && !validTargets.find(t => t.value === this.routeForm.target_protocol)) {
                    this.routeForm.target_protocol = validTargets[0].value;
                }
                this.onTargetProtocolChange();
            },

            onTargetProtocolChange() {
                this.targetModels = this.getModelsForProtocol(this.routeForm.target_protocol);
            },

            openRouteModal(route = null) {
                if (route) {
                    const mappings = Array.isArray(route.model_mappings) && route.model_mappings.length > 0
                        ? JSON.parse(JSON.stringify(route.model_mappings))
                        : [{ match: '', target: '' }];
                    this.routeForm = {
                        id: route.id,
                        source_protocol: route.source_protocol || 'openai',
                        target_protocol: route.target_protocol || 'openai',
                        model_mappings: mappings,
                        status: route.status
                    };
                    this.routeModal = { show: true, isEdit: true };
                } else {
                    this.routeForm = {
                        id: 0,
                        source_protocol: 'openai',
                        target_protocol: 'openai',
                        model_mappings: [{ match: '', target: '' }],
                        status: 1
                    };
                    this.routeModal = { show: true, isEdit: false };
                }
                this.onSourceProtocolChange();
                this.onTargetProtocolChange();
            },

            addMapping() {
                this.routeForm.model_mappings.push({ match: '', target: '' });
            },

            removeMapping(index) {
                if (this.routeForm.model_mappings.length > 1) {
                    this.routeForm.model_mappings.splice(index, 1);
                }
            },

            async saveRoute() {
                const gStore = Alpine.store('global');
                const validMappings = this.routeForm.model_mappings.filter(m => m.match.trim() !== '');
                if (validMappings.length === 0) {
                    gStore.showToast(gStore.t('err_empty_mapping'), 'error');
                    return;
                }
                if (!this.routeForm.source_protocol || !this.routeForm.target_protocol) {
                    gStore.showToast(gStore.t('err_empty_protocols'), 'error');
                    return;
                }

                try {
                    const method = this.routeModal.isEdit ? 'PUT' : 'POST';
                    const payload = {
                        id: this.routeForm.id,
                        source_protocol: this.routeForm.source_protocol,
                        target_protocol: this.routeForm.target_protocol,
                        model_mappings: validMappings,
                        status: this.routeForm.status
                    };
                    const res = await fetch('/api/admin/routes', {
                        method,
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify(payload)
                    });
                    if (res.ok) {
                        gStore.showToast(this.routeModal.isEdit ? gStore.t('route_updated') : gStore.t('route_added'));
                        this.routeModal.show = false;
                        this.fetchRoutes();
                    } else {
                        const err = await res.text();
                        gStore.showToast(gStore.t('save_failed') + ': ' + err, 'error');
                    }
                } catch(e) {
                    gStore.showToast(gStore.t('network_error'), 'error');
                }
            },

            async deleteRoute(id) {
                const gStore = Alpine.store('global');
                if(!confirm(gStore.lang === 'zh' ? '确定要删除这个路由吗？' : 'Are you sure you want to delete this route?')) return;
                try {
                    const res = await fetch(`/api/admin/routes?id=${id}`, { method: 'DELETE' });
                    if (res.ok) {
                        gStore.showToast(gStore.t('route_deleted'));
                        this.fetchRoutes();
                    } else {
                        gStore.showToast(gStore.t('delete_failed'), 'error');
                    }
                } catch(e) {
                    gStore.showToast(gStore.t('network_error'), 'error');
                }
            },

            init() {
                this.fetchRoutes();
                this.fetchAllModels();

                this.$watch('$store.global.currentTab', (newTab) => {
                    if (newTab === 'rules') {
                        this.fetchRoutes();
                        this.fetchAllModels();
                    }
                });
                
                this.$watch('routeForm.source_protocol', () => {
                    this.onSourceProtocolChange();
                });
                this.$watch('routeForm.target_protocol', () => {
                    this.onTargetProtocolChange();
                });
            }
        };
    },
    template: `
        <div x-show="$store.global.currentTab === 'rules'" class="max-w-6xl mx-auto w-full">
            <div class="flex justify-between items-center mb-6">
                <div>
                    <h2 class="text-3xl font-bold" x-text="$store.global.t('tab_routes_title')"></h2>
                    <p class="text-base-content/60 text-sm mt-2" x-text="$store.global.t('routes_subtitle')"></p>
                </div>
                <button @click="openRouteModal()" class="btn btn-secondary shadow-lg shadow-secondary/20">
                    <span class="text-lg">+</span> <span x-text="$store.global.t('btn_add_new_route')"></span>
                </button>
            </div>
            
            <div class="card bg-base-100 shadow overflow-x-auto">
                <table class="table table-zebra w-full">
                    <thead>
                        <tr>
                            <th x-text="$store.global.t('route_header_source')"></th>
                            <th x-text="$store.global.t('route_header_target')"></th>
                            <th x-text="$store.global.t('route_header_mapping')"></th>
                            <th class="text-center" x-text="$store.global.t('table_status')"></th>
                            <th class="text-right" x-text="$store.global.t('actions')"></th>
                        </tr>
                    </thead>
                    <tbody>
                        <template x-for="route in $store.global.routes" :key="route.id">
                            <tr>
                                <td>
                                    <span :class="$store.global.protocolBadge(route.source_protocol)" class="badge badge-sm font-bold uppercase" x-text="$store.global.protocolLabel(route.source_protocol)"></span>
                                </td>
                                <td>
                                    <div class="text-sm font-medium" :class="$store.global.protocolClass(route.target_protocol)" x-text="$store.global.protocolLabel(route.target_protocol)"></div>
                                    <div class="text-xs text-base-content/50 mt-0.5" x-text="getDescShort(route.source_protocol, route.target_protocol)"></div>
                                </td>
                                <td>
                                    <div class="flex flex-wrap gap-1.5">
                                        <template x-for="(m, i) in route.model_mappings" :key="i">
                                            <span class="badge badge-outline gap-1">
                                                <span class="text-info font-mono" x-text="m.match"></span>
                                                <span class="text-base-content/50 mx-1">→</span>
                                                <span class="text-success font-mono" x-text="m.target"></span>
                                            </span>
                                        </template>
                                        <template x-if="!route.model_mappings || route.model_mappings.length === 0">
                                            <span class="text-base-content/50 text-xs" x-text="$store.global.t('no_mapping')"></span>
                                        </template>
                                    </div>
                                </td>
                                <td class="text-center">
                                    <template x-if="route.status === 1"><span class="badge badge-success badge-sm" x-text="$store.global.t('status_enabled_short')"></span></template>
                                    <template x-if="route.status !== 1"><span class="badge badge-ghost badge-sm" x-text="$store.global.t('status_disabled_short')"></span></template>
                                </td>
                                <td class="text-right space-x-2">
                                    <button @click="openRouteModal(route)" class="btn btn-ghost btn-xs text-info" x-text="$store.global.t('edit')"></button>
                                    <button @click="deleteRoute(route.id)" class="btn btn-ghost btn-xs text-error" x-text="$store.global.t('delete')"></button>
                                </td>
                            </tr>
                        </template>
                        <template x-if="$store.global.routes.length === 0">
                            <tr>
                                <td colspan="5" class="text-center py-8 text-base-content/50" x-text="$store.global.t('no_routes')"></td>
                            </tr>
                        </template>
                    </tbody>
                </table>
            </div>

            <!-- Route Editor Modal -->
            <dialog class="modal" :class="routeModal.show ? 'modal-open' : ''">
                <div class="modal-box w-11/12 max-w-2xl">
                    <button class="btn btn-sm btn-circle btn-ghost absolute right-2 top-2" @click="routeModal.show = false">✕</button>
                    <h3 class="font-bold text-lg mb-6" x-text="routeModal.isEdit ? $store.global.t('edit_route') : $store.global.t('add_new_route')"></h3>
                    
                    <div class="space-y-6">
                        <div class="grid grid-cols-2 gap-4">
                            <label class="form-control w-full">
                                <div class="label"><span class="label-text font-medium" x-text="$store.global.t('label_source_req')"></span></div>
                                <select x-model="routeForm.source_protocol" class="select select-bordered select-sm w-full">
                                    <option value="anthropic">Anthropic — Messages API</option>
                                    <option value="openai">OpenAI — Chat Completions API</option>
                                    <option value="google" x-text="$store.global.t('option_google_geap')"></option>
                                </select>
                                <div class="label"><span class="label-text-alt text-base-content/50" x-text="$store.global.t('hint_client_protocol')"></span></div>
                            </label>
                            <label class="form-control w-full">
                                <div class="label"><span class="label-text font-medium" x-text="$store.global.t('label_target_req')"></span></div>
                                <select x-model="routeForm.target_protocol" class="select select-bordered select-sm w-full">
                                    <template x-for="tp in availableTargetProtocols" :key="tp.value">
                                        <option :value="tp.value" x-text="tp.label"></option>
                                    </template>
                                </select>
                                <div class="label">
                                    <template x-if="routeTypeDesc"><span class="label-text-alt text-success" x-text="routeTypeDesc"></span></template>
                                    <template x-if="!routeTypeDesc"><span class="label-text-alt text-base-content/50" x-text="$store.global.t('hint_upstream_protocol')"></span></template>
                                </div>
                            </label>
                        </div>

                        <div class="border-t border-base-300 pt-4">
                            <div class="flex justify-between items-center mb-3">
                                <span class="label-text font-medium" x-text="$store.global.t('label_mappings_req')"></span>
                                <button @click="addMapping()" class="btn btn-outline btn-info btn-xs" x-text="$store.global.t('btn_add_mapping_simple')"></button>
                            </div>
                            <p class="text-xs text-base-content/50 mb-3" x-text="$store.global.t('hint_mapping_desc')"></p>
                            
                            <div class="space-y-2">
                                <template x-for="(mapping, index) in routeForm.model_mappings" :key="index">
                                    <div class="flex items-center gap-2 bg-base-200 rounded-lg p-2 border border-base-300">
                                        <span class="text-base-content/50 text-xs w-5" x-text="index + 1 + '.'"></span>
                                        <input x-model="mapping.match" type="text" :placeholder="$store.global.t('placeholder_match_model')" 
                                            :list="'match-list-' + index"
                                            class="input input-sm input-bordered flex-1 text-info font-mono">
                                        <datalist :id="'match-list-' + index">
                                            <option value="*" x-text="$store.global.t('option_all_models')"></option>
                                            <template x-for="m in sourceModels" :key="m.name"><option :value="m.name" x-text="m.display_name"></option></template>
                                        </datalist>
                                        
                                        <span class="text-base-content/40 text-sm">→</span>
                                        
                                        <input x-model="mapping.target" type="text" :placeholder="$store.global.t('placeholder_target_model')" 
                                            :list="'target-list-' + index"
                                            class="input input-sm input-bordered flex-1 text-success font-mono">
                                        <datalist :id="'target-list-' + index">
                                            <template x-for="m in targetModels" :key="m.name"><option :value="m.name" x-text="m.display_name"></option></template>
                                        </datalist>
                                        
                                        <button @click="removeMapping(index)" class="btn btn-ghost btn-xs text-error" :disabled="routeForm.model_mappings.length <= 1">✕</button>
                                    </div>
                                </template>
                            </div>
                        </div>

                        <template x-if="$store.global.proMode">
                            <label class="form-control w-full">
                                <div class="label"><span class="label-text" x-text="$store.global.t('label_route_status')"></span></div>
                                <select x-model="routeForm.status" class="select select-bordered select-sm w-full">
                                    <option value="1" x-text="$store.global.t('status_enabled_short')"></option>
                                    <option value="0" x-text="$store.global.t('status_disabled_short')"></option>
                                </select>
                            </label>
                        </template>
                    </div>
                    
                    <div class="modal-action mt-6">
                        <button class="btn" @click="routeModal.show = false" x-text="$store.global.t('cancel')"></button>
                        <button class="btn btn-secondary shadow-lg shadow-secondary/20" @click="saveRoute()" x-text="$store.global.t('btn_save_simple')"></button>
                    </div>
                </div>
            </dialog>
        </div>
    `
};
