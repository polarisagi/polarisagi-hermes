export default {
    name: 'rulesComponent',
    setup() {
        return {
            routeModal: { show: false, isEdit: false },
            routeForm: {
                id: 0,
                requested_model_id: '',
                target_user_model_id: 0,
                is_active: true
            },

            async fetchRoutes() {
                if (Alpine.store('global').currentTab !== 'rules') return;
                try {
                    const res = await fetch('/api/admin/routes');
                    const data = await res.json() || [];
                    Alpine.store('global').routes = data;
                } catch (e) { console.error(e) }
            },

            async fetchAllModels() {
                try {
                    const res = await fetch('/api/admin/models');
                    const json = await res.json() || [];
                    Alpine.store('global').allModels = Array.isArray(json) ? json : [];
                } catch (e) { console.error(e) }
            },

            getTargetModelName(id) {
                const models = Alpine.store('global').allModels || [];
                const m = models.find(x => x.id === id);
                return m ? `${m.display_name} (${m.actual_model_id})` : `ID: ${id}`;
            },

            openRouteModal(route = null) {
                if (route) {
                    this.routeForm = {
                        id: route.id,
                        requested_model_id: route.requested_model_id || '',
                        target_user_model_id: route.target_user_model_id || 0,
                        is_active: route.is_active
                    };
                    this.routeModal = { show: true, isEdit: true };
                } else {
                    const models = Alpine.store('global').allModels || [];
                    this.routeForm = {
                        id: 0,
                        requested_model_id: '',
                        target_user_model_id: models.length > 0 ? models[0].id : 0,
                        is_active: true
                    };
                    this.routeModal = { show: true, isEdit: false };
                }
            },

            async saveRoute() {
                const gStore = Alpine.store('global');
                if (!this.routeForm.requested_model_id.trim()) {
                    gStore.showToast(gStore.t('err_empty_mapping') || 'Source model cannot be empty', 'error');
                    return;
                }
                if (!this.routeForm.target_user_model_id) {
                    gStore.showToast(gStore.t('err_empty_protocols') || 'Target model cannot be empty', 'error');
                    return;
                }

                try {
                    const method = this.routeModal.isEdit ? 'PUT' : 'POST';
                    const payload = {
                        id: this.routeForm.id,
                        requested_model_id: this.routeForm.requested_model_id.trim(),
                        target_user_model_id: parseInt(this.routeForm.target_user_model_id, 10),
                        is_active: this.routeForm.is_active === true || this.routeForm.is_active === 'true' || this.routeForm.is_active === 1
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
                            <th x-text="$store.global.t('route_header_source') || 'Requested Model (Match)'"></th>
                            <th x-text="$store.global.t('route_header_target') || 'Target User Model'"></th>
                            <th class="text-center" x-text="$store.global.t('table_status')"></th>
                            <th class="text-right" x-text="$store.global.t('actions')"></th>
                        </tr>
                    </thead>
                    <tbody>
                        <template x-for="route in $store.global.routes" :key="route.id">
                            <tr>
                                <td>
                                    <div class="font-mono text-info font-bold" x-text="route.requested_model_id"></div>
                                </td>
                                <td>
                                    <div class="text-sm font-medium text-success font-mono" x-text="getTargetModelName(route.target_user_model_id)"></div>
                                </td>
                                <td class="text-center">
                                    <template x-if="route.is_active"><span class="badge badge-success badge-sm" x-text="$store.global.t('status_enabled_short')"></span></template>
                                    <template x-if="!route.is_active"><span class="badge badge-ghost badge-sm" x-text="$store.global.t('status_disabled_short')"></span></template>
                                </td>
                                <td class="text-right space-x-2">
                                    <button @click="openRouteModal(route)" class="btn btn-ghost btn-xs text-info" x-text="$store.global.t('edit')"></button>
                                    <button @click="deleteRoute(route.id)" class="btn btn-ghost btn-xs text-error" x-text="$store.global.t('delete')"></button>
                                </td>
                            </tr>
                        </template>
                        <template x-if="!$store.global.routes || $store.global.routes.length === 0">
                            <tr>
                                <td colspan="4" class="text-center py-8 text-base-content/50" x-text="$store.global.t('no_routes')"></td>
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
                        <label class="form-control w-full">
                            <div class="label"><span class="label-text font-medium" x-text="$store.global.t('label_source_req') || 'Requested Model (String or Regex)'"></span></div>
                            <input x-model="routeForm.requested_model_id" type="text" class="input input-bordered w-full font-mono text-info" placeholder="e.g. gpt-4o or ^claude-.*" />
                            <div class="label"><span class="label-text-alt text-base-content/50" x-text="$store.global.t('hint_client_protocol') || 'The model name requested by the client.'"></span></div>
                        </label>
                        
                        <label class="form-control w-full">
                            <div class="label"><span class="label-text font-medium" x-text="$store.global.t('label_target_req') || 'Target User Model'"></span></div>
                            <select x-model="routeForm.target_user_model_id" class="select select-bordered w-full font-mono text-success">
                                <template x-for="m in $store.global.allModels" :key="m.id">
                                    <option :value="m.id" x-text="m.display_name + ' (' + m.actual_model_id + ')'"></option>
                                </template>
                            </select>
                            <div class="label"><span class="label-text-alt text-base-content/50" x-text="$store.global.t('hint_upstream_protocol') || 'The actual underlying model configured in Channels.'"></span></div>
                        </label>

                        <template x-if="$store.global.proMode">
                            <label class="form-control w-full">
                                <div class="label"><span class="label-text" x-text="$store.global.t('label_route_status')"></span></div>
                                <select x-model="routeForm.is_active" class="select select-bordered select-sm w-full">
                                    <option value="true" x-text="$store.global.t('status_enabled_short')"></option>
                                    <option value="false" x-text="$store.global.t('status_disabled_short')"></option>
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
