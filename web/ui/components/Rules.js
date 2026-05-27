export default {
    name: 'rulesComponent',
    setup() {
        return {
            routeModal: { show: false, isEdit: false },

            // sourceMode: 'tier' | 'wildcard' | 'custom'
            sourceMode: 'tier',
            selectedTier: 'smart',

            routeForm: {
                id: 0,
                requested_model_id: '',
                target_user_model_id: 0,
                is_active: true
            },

            // ─── Data Fetching ───────────────────────────────────────────────

            async fetchRoutes() {
                if (Alpine.store('global').currentTab !== 'rules') return;
                try {
                    const res = await fetch('/api/admin/routes');
                    const data = await res.json() || [];
                    Alpine.store('global').routes = data;
                } catch (e) { console.error(e); }
            },

            async fetchAllModels() {
                try {
                    const res = await fetch('/api/admin/models');
                    const json = await res.json() || [];
                    Alpine.store('global').allModels = Array.isArray(json) ? json : [];
                } catch (e) { console.error(e); }
            },

            // ─── Helpers ─────────────────────────────────────────────────────

            getTargetModelName(id) {
                const models = Alpine.store('global').allModels || [];
                const m = models.find(x => x.id === id);
                return m ? `${m.display_name || m.model_id} (${m.model_id})` : `ID: ${id}`;
            },

            getTierLabel(tier) {
                const gStore = Alpine.store('global');
                const map = {
                    smart:     gStore.t('tier_smart')     || '🏆 旗舰型 (smart)',
                    fast:      gStore.t('tier_fast')      || '⚡ 极速型 (fast)',
                    reasoning: gStore.t('tier_reasoning') || '🧠 沉思型 (reasoning)',
                    '*':       gStore.t('tier_wildcard')  || '✸ 全部模型 (*)',
                };
                return map[tier] || tier;
            },

            getTierBadgeClass(tier) {
                const map = {
                    smart:     'badge-warning',
                    fast:      'badge-info',
                    reasoning: 'badge-secondary',
                    '*':       'badge-ghost',
                };
                return map[tier] || 'badge-neutral';
            },

            getModelTierBadge(tier) {
                const map = {
                    smart:     'badge-warning',
                    fast:      'badge-info',
                    reasoning: 'badge-secondary',
                };
                return map[tier] || 'badge-ghost';
            },

            // Returns display label for the source column in the table
            getSourceLabel(requestedModelId) {
                if (!requestedModelId) return '?';
                if (requestedModelId === '*') return '✸ *';
                if (['smart', 'fast', 'reasoning'].includes(requestedModelId)) {
                    return this.getTierLabel(requestedModelId);
                }
                return requestedModelId;
            },

            isSourceTier(requested) {
                return ['smart', 'fast', 'reasoning'].includes(requested);
            },

            // ─── Modal Logic ─────────────────────────────────────────────────

            openRouteModal(route = null) {
                const models = Alpine.store('global').allModels || [];
                if (route) {
                    const rid = route.requested_model_id || '';
                    // Determine sourceMode from existing value
                    if (rid === '*') {
                        this.sourceMode = 'wildcard';
                        this.selectedTier = 'smart';
                    } else if (['smart', 'fast', 'reasoning'].includes(rid)) {
                        this.sourceMode = 'tier';
                        this.selectedTier = rid;
                    } else {
                        this.sourceMode = 'custom';
                        this.selectedTier = 'smart';
                    }
                    this.routeForm = {
                        id: route.id,
                        requested_model_id: rid,
                        target_user_model_id: route.target_user_model_id || 0,
                        is_active: route.is_active
                    };
                    this.routeModal = { show: true, isEdit: true };
                } else {
                    this.sourceMode = 'tier';
                    this.selectedTier = 'smart';
                    this.routeForm = {
                        id: 0,
                        requested_model_id: 'smart',
                        target_user_model_id: models.length > 0 ? models[0].id : 0,
                        is_active: true
                    };
                    this.routeModal = { show: true, isEdit: false };
                }
            },

            // Called when user clicks a source mode tab
            setSourceMode(mode) {
                this.sourceMode = mode;
                if (mode === 'tier') {
                    this.routeForm.requested_model_id = this.selectedTier || 'smart';
                } else if (mode === 'wildcard') {
                    this.routeForm.requested_model_id = '*';
                } else {
                    // custom — keep whatever was typed, reset if it was a tier/wildcard
                    if (['smart', 'fast', 'reasoning', '*'].includes(this.routeForm.requested_model_id)) {
                        this.routeForm.requested_model_id = '';
                    }
                }
            },

            setTier(tier) {
                this.selectedTier = tier;
                this.routeForm.requested_model_id = tier;
            },

            // ─── CRUD ─────────────────────────────────────────────────────────

            async saveRoute() {
                const gStore = Alpine.store('global');

                // Resolve requested_model_id
                let reqModel = this.routeForm.requested_model_id;
                if (this.sourceMode === 'tier') {
                    reqModel = this.selectedTier;
                } else if (this.sourceMode === 'wildcard') {
                    reqModel = '*';
                } else {
                    reqModel = (reqModel || '').trim();
                }

                if (!reqModel) {
                    gStore.showToast(gStore.t('err_empty_mapping') || '源模型不能为空', 'error');
                    return;
                }
                if (!this.routeForm.target_user_model_id) {
                    gStore.showToast(gStore.t('err_empty_protocols') || '目标模型不能为空', 'error');
                    return;
                }

                try {
                    const method = this.routeModal.isEdit ? 'PUT' : 'POST';
                    const payload = {
                        id: this.routeForm.id,
                        requested_model_id: reqModel,
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
                } catch (e) {
                    gStore.showToast(gStore.t('network_error'), 'error');
                }
            },

            async deleteRoute(id) {
                const gStore = Alpine.store('global');
                if (!confirm(gStore.lang === 'zh' ? '确定要删除这个转发规则吗？' : 'Delete this rule?')) return;
                try {
                    const res = await fetch(`/api/admin/routes?id=${id}`, { method: 'DELETE' });
                    if (res.ok) {
                        gStore.showToast(gStore.t('route_deleted'));
                        this.fetchRoutes();
                    } else {
                        gStore.showToast(gStore.t('delete_failed'), 'error');
                    }
                } catch (e) {
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

        <!-- ── Header ─────────────────────────────────── -->
        <div class="flex justify-between items-center mb-6">
            <div>
                <h2 class="text-3xl font-bold" x-text="$store.global.t('tab_routes_title')"></h2>
                <p class="text-base-content/60 text-sm mt-1" x-text="$store.global.t('routes_subtitle')"></p>
            </div>
            <button @click="openRouteModal()" class="btn btn-secondary shadow-lg shadow-secondary/20">
                <span class="text-lg">+</span>
                <span x-text="$store.global.t('btn_add_new_route')"></span>
            </button>
        </div>

        <!-- ── Mode Banner ────────────────────────────── -->
        <template x-if="!$store.global.proMode">
            <div class="alert mb-4 bg-base-200/60 border border-base-300/50 py-2.5">
                <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 shrink-0 opacity-60" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
                <span class="text-xs text-base-content/60" x-text="$store.global.t('routes_simple_hint') || '极简模式：按模型能力梯队智能转发。Pro 模式可配置精确的模型 ID 1:1 映射。'"></span>
            </div>
        </template>
        <template x-if="$store.global.proMode">
            <div class="alert mb-4 bg-primary/5 border border-primary/20 py-2.5">
                <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 shrink-0 text-primary" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4M7.835 4.697a3.42 3.42 0 001.946-.806 3.42 3.42 0 014.438 0 3.42 3.42 0 001.946.806 3.42 3.42 0 013.138 3.138 3.42 3.42 0 00.806 1.946 3.42 3.42 0 010 4.438 3.42 3.42 0 00-.806 1.946 3.42 3.42 0 01-3.138 3.138 3.42 3.42 0 00-1.946.806 3.42 3.42 0 01-4.438 0 3.42 3.42 0 00-1.946-.806 3.42 3.42 0 01-3.138-3.138 3.42 3.42 0 00-.806-1.946 3.42 3.42 0 010-4.438 3.42 3.42 0 00.806-1.946 3.42 3.42 0 013.138-3.138z"/></svg>
                <span class="text-xs text-primary/80" x-text="$store.global.t('routes_pro_hint') || 'Pro 模式：精确配置模型 ID → 目标模型的 1:1 强制映射，优先级高于智能推断。支持精确匹配、正则表达式和通配符 * 兜底。'"></span>
            </div>
        </template>

        <!-- ── Rules Table ────────────────────────────── -->
        <div class="card bg-base-100 shadow overflow-x-auto">
            <table class="table table-zebra w-full">
                <thead>
                    <tr>
                        <th x-text="$store.global.t('route_header_source') || '匹配规则'"></th>
                        <th x-text="$store.global.t('route_header_target') || '转发目标'"></th>
                        <th class="text-center" x-text="$store.global.t('table_status')"></th>
                        <th class="text-right" x-text="$store.global.t('actions')"></th>
                    </tr>
                </thead>
                <tbody>
                    <template x-for="route in $store.global.routes" :key="route.id">
                        <tr>
                            <td>
                                <!-- Tier badge -->
                                <template x-if="isSourceTier(route.requested_model_id)">
                                    <div class="flex items-center gap-2">
                                        <span class="badge badge-sm font-mono font-semibold"
                                              :class="getTierBadgeClass(route.requested_model_id)"
                                              x-text="getTierLabel(route.requested_model_id)"></span>
                                    </div>
                                </template>
                                <!-- Wildcard -->
                                <template x-if="route.requested_model_id === '*'">
                                    <div class="flex items-center gap-2">
                                        <span class="badge badge-ghost badge-sm font-mono">✸ * 全部</span>
                                    </div>
                                </template>
                                <!-- Custom / Regex -->
                                <template x-if="!isSourceTier(route.requested_model_id) && route.requested_model_id !== '*'">
                                    <div class="font-mono text-info text-sm font-semibold" x-text="route.requested_model_id"></div>
                                </template>
                            </td>
                            <td>
                                <div class="text-sm font-medium text-success font-mono"
                                     x-text="getTargetModelName(route.target_user_model_id)"></div>
                            </td>
                            <td class="text-center">
                                <template x-if="route.is_active">
                                    <span class="badge badge-success badge-sm" x-text="$store.global.t('status_enabled_short')"></span>
                                </template>
                                <template x-if="!route.is_active">
                                    <span class="badge badge-ghost badge-sm" x-text="$store.global.t('status_disabled_short')"></span>
                                </template>
                            </td>
                            <td class="text-right space-x-2">
                                <button @click="openRouteModal(route)" class="btn btn-ghost btn-xs text-info" x-text="$store.global.t('edit')"></button>
                                <button @click="deleteRoute(route.id)" class="btn btn-ghost btn-xs text-error" x-text="$store.global.t('delete')"></button>
                            </td>
                        </tr>
                    </template>
                    <template x-if="!$store.global.routes || $store.global.routes.length === 0">
                        <tr>
                            <td colspan="4" class="text-center py-10 text-base-content/40">
                                <div class="text-3xl mb-2">🔀</div>
                                <div x-text="$store.global.t('no_routes')"></div>
                            </td>
                        </tr>
                    </template>
                </tbody>
            </table>
        </div>

        <!-- ══════════════════════════════════════════════
             Route Editor Modal
             ══════════════════════════════════════════════ -->
        <dialog class="modal" :class="routeModal.show ? 'modal-open' : ''">
            <div class="modal-box w-11/12 max-w-2xl">
                <button class="btn btn-sm btn-circle btn-ghost absolute right-2 top-2"
                        @click="routeModal.show = false">✕</button>

                <h3 class="font-bold text-lg mb-1"
                    x-text="routeModal.isEdit ? $store.global.t('edit_route') : $store.global.t('add_new_route')"></h3>
                <p class="text-xs text-base-content/50 mb-6"
                   x-text="$store.global.proMode
                       ? ($store.global.t('route_modal_pro_desc') || '精确配置：源模型 ID → 目标模型 ID 的强制映射规则')
                       : ($store.global.t('route_modal_simple_desc') || '选择客户端请求的模型类型，系统将自动选择目标渠道中对应能力的模型')"></p>

                <div class="space-y-6">

                    <!-- ── SOURCE: Simple Mode ───────────────────── -->
                    <template x-if="!$store.global.proMode">
                        <div class="form-control w-full">
                            <div class="label pb-1">
                                <span class="label-text font-semibold" x-text="$store.global.t('label_source_req') || '源协议 (接收端) *'"></span>
                            </div>

                            <!-- Source Mode Tabs -->
                            <div class="tabs tabs-boxed bg-base-200 p-1 mb-3 gap-1">
                                <a class="tab tab-sm flex-1 gap-1 transition-all"
                                   :class="sourceMode === 'tier' ? 'tab-active' : ''"
                                   @click="setSourceMode('tier')">
                                    <span>🎯</span>
                                    <span x-text="$store.global.t('source_mode_tier') || '按能力梯队'"></span>
                                </a>
                                <a class="tab tab-sm flex-1 gap-1 transition-all"
                                   :class="sourceMode === 'wildcard' ? 'tab-active' : ''"
                                   @click="setSourceMode('wildcard')">
                                    <span>✸</span>
                                    <span x-text="$store.global.t('source_mode_wildcard') || '全部模型 (*)'"></span>
                                </a>
                                <a class="tab tab-sm flex-1 gap-1 transition-all"
                                   :class="sourceMode === 'custom' ? 'tab-active' : ''"
                                   @click="setSourceMode('custom')">
                                    <span>✏️</span>
                                    <span x-text="$store.global.t('source_mode_custom') || '自定义'"></span>
                                </a>
                            </div>

                            <!-- Tier Selector -->
                            <template x-if="sourceMode === 'tier'">
                                <div class="space-y-2">
                                    <div class="grid grid-cols-3 gap-2">
                                        <!-- Smart -->
                                        <button type="button"
                                                @click="setTier('smart')"
                                                :class="selectedTier === 'smart'
                                                    ? 'ring-2 ring-warning ring-offset-2 ring-offset-base-100 bg-warning/10 border-warning'
                                                    : 'border-base-300 hover:border-warning/50'"
                                                class="btn btn-outline border-2 flex-col h-auto py-3 gap-1 transition-all">
                                            <span class="text-xl">🏆</span>
                                            <span class="font-bold text-sm" x-text="$store.global.t('tier_smart_label') || '旗舰型'"></span>
                                            <span class="font-mono text-xs opacity-60">smart</span>
                                        </button>
                                        <!-- Fast -->
                                        <button type="button"
                                                @click="setTier('fast')"
                                                :class="selectedTier === 'fast'
                                                    ? 'ring-2 ring-info ring-offset-2 ring-offset-base-100 bg-info/10 border-info'
                                                    : 'border-base-300 hover:border-info/50'"
                                                class="btn btn-outline border-2 flex-col h-auto py-3 gap-1 transition-all">
                                            <span class="text-xl">⚡</span>
                                            <span class="font-bold text-sm" x-text="$store.global.t('tier_fast_label') || '极速型'"></span>
                                            <span class="font-mono text-xs opacity-60">fast</span>
                                        </button>
                                        <!-- Reasoning -->
                                        <button type="button"
                                                @click="setTier('reasoning')"
                                                :class="selectedTier === 'reasoning'
                                                    ? 'ring-2 ring-secondary ring-offset-2 ring-offset-base-100 bg-secondary/10 border-secondary'
                                                    : 'border-base-300 hover:border-secondary/50'"
                                                class="btn btn-outline border-2 flex-col h-auto py-3 gap-1 transition-all">
                                            <span class="text-xl">🧠</span>
                                            <span class="font-bold text-sm" x-text="$store.global.t('tier_reasoning_label') || '沉思型'"></span>
                                            <span class="font-mono text-xs opacity-60">reasoning</span>
                                        </button>
                                    </div>
                                    <!-- Tier description -->
                                    <div class="rounded-lg bg-base-200/60 px-3 py-2 text-xs text-base-content/60">
                                        <template x-if="selectedTier === 'smart'">
                                            <span x-text="$store.global.t('tier_smart_desc') || '匹配所有旗舰模型请求 (如 gpt-4o, claude-3-5-sonnet, gemini-2.5-pro)，转发到目标渠道中同为 smart 梯队的模型'"></span>
                                        </template>
                                        <template x-if="selectedTier === 'fast'">
                                            <span x-text="$store.global.t('tier_fast_desc') || '匹配所有极速轻量模型请求 (如 gpt-4o-mini, claude-3-haiku, gemini-2.5-flash)，转发到目标渠道中同为 fast 梯队的模型'"></span>
                                        </template>
                                        <template x-if="selectedTier === 'reasoning'">
                                            <span x-text="$store.global.t('tier_reasoning_desc') || '匹配所有深度推理模型请求 (如 o1, o3-mini, DeepSeek-R1)，转发到目标渠道中同为 reasoning 梯队的模型'"></span>
                                        </template>
                                    </div>
                                </div>
                            </template>

                            <!-- Wildcard -->
                            <template x-if="sourceMode === 'wildcard'">
                                <div class="flex items-center gap-3 rounded-lg bg-base-200/60 px-4 py-3 border border-base-300">
                                    <span class="text-2xl">✸</span>
                                    <div>
                                        <div class="font-mono font-bold text-sm">* (全部模型)</div>
                                        <div class="text-xs text-base-content/50 mt-0.5"
                                             x-text="$store.global.t('wildcard_desc') || '所有未被其他规则精确命中的模型请求，都将被引流至此目标模型'"></div>
                                    </div>
                                </div>
                            </template>

                            <!-- Custom text input -->
                            <template x-if="sourceMode === 'custom'">
                                <div class="space-y-2">
                                    <input x-model="routeForm.requested_model_id"
                                           type="text"
                                           class="input input-bordered w-full font-mono text-info"
                                           :placeholder="$store.global.t('placeholder_custom_source') || 'e.g. gpt-4o 或正则 ^claude-.*'" />
                                    <div class="text-xs text-base-content/50"
                                         x-text="$store.global.t('custom_source_hint') || '支持精确模型名（如 gpt-4o）或正则表达式（如 ^claude-.*）'"></div>
                                </div>
                            </template>

                            <div class="label pt-1">
                                <span class="label-text-alt text-base-content/40"
                                      x-text="$store.global.t('hint_client_protocol') || '客户端通过哪个协议接入网关'"></span>
                            </div>
                        </div>
                    </template>

                    <!-- ── SOURCE: Pro Mode ──────────────────────── -->
                    <template x-if="$store.global.proMode">
                        <div class="form-control w-full">
                            <div class="label pb-1">
                                <span class="label-text font-semibold" x-text="$store.global.t('label_source_req') || '源模型 ID *'"></span>
                                <span class="label-text-alt">
                                    <div class="flex gap-1">
                                        <button type="button"
                                                @click="routeForm.requested_model_id = '*'"
                                                class="badge badge-ghost badge-sm cursor-pointer hover:badge-warning font-mono transition-colors">* 通配符</button>
                                        <button type="button"
                                                @click="routeForm.requested_model_id = 'smart'"
                                                class="badge badge-warning badge-sm cursor-pointer hover:opacity-80 font-mono transition-colors">smart</button>
                                        <button type="button"
                                                @click="routeForm.requested_model_id = 'fast'"
                                                class="badge badge-info badge-sm cursor-pointer hover:opacity-80 font-mono transition-colors">fast</button>
                                        <button type="button"
                                                @click="routeForm.requested_model_id = 'reasoning'"
                                                class="badge badge-secondary badge-sm cursor-pointer hover:opacity-80 font-mono transition-colors">reasoning</button>
                                    </div>
                                </span>
                            </div>
                            <input x-model="routeForm.requested_model_id"
                                   type="text"
                                   class="input input-bordered w-full font-mono text-info"
                                   :placeholder="$store.global.t('placeholder_pro_source') || 'e.g. gpt-4o 或 ^claude-.* 或 * 或 smart'" />
                            <div class="label">
                                <span class="label-text-alt text-base-content/40"
                                      x-text="$store.global.t('hint_pro_source') || '支持精确模型名、正则、能力梯队关键字(smart/fast/reasoning)、通配符(*)'"></span>
                            </div>
                        </div>
                    </template>

                    <!-- ── TARGET MODEL ──────────────────────────── -->
                    <div class="form-control w-full">
                        <div class="label pb-1">
                            <span class="label-text font-semibold" x-text="$store.global.t('label_target_req') || '目标协议 (转发端) *'"></span>
                        </div>

                        <template x-if="$store.global.allModels && $store.global.allModels.length > 0">
                            <div class="space-y-2">
                                <select x-model="routeForm.target_user_model_id"
                                        class="select select-bordered w-full font-mono text-success">
                                    <template x-for="m in $store.global.allModels" :key="m.id">
                                        <option :value="m.id"
                                                x-text="(m.display_name || m.model_id) + ' (' + m.model_id + ') [' + (m.capability_tier || 'smart') + ']'"></option>
                                    </template>
                                </select>
                                <!-- Selected model preview -->
                                <template x-if="routeForm.target_user_model_id">
                                    <div class="flex items-center gap-2 px-3 py-2 rounded-lg bg-base-200/60 border border-base-300/50">
                                        <span class="text-xs text-base-content/50" x-text="$store.global.t('selected') || '已选：'"></span>
                                        <template x-for="m in $store.global.allModels.filter(x => x.id == routeForm.target_user_model_id)" :key="m.id">
                                            <div class="flex items-center gap-2">
                                                <span class="badge badge-sm font-mono"
                                                      :class="getModelTierBadge(m.capability_tier)"
                                                      x-text="m.capability_tier || 'smart'"></span>
                                                <span class="font-mono text-success text-sm font-bold" x-text="m.model_id"></span>
                                                <span class="text-xs text-base-content/40" x-text="'(ID: ' + m.id + ')'"></span>
                                            </div>
                                        </template>
                                    </div>
                                </template>
                            </div>
                        </template>

                        <template x-if="!$store.global.allModels || $store.global.allModels.length === 0">
                            <div class="flex items-center gap-2 p-3 rounded-lg bg-warning/10 border border-warning/30 text-warning text-sm">
                                <span>⚠️</span>
                                <span x-text="$store.global.t('no_models_hint') || '请先在「渠道账号」中添加渠道和模型，再配置转发规则'"></span>
                            </div>
                        </template>

                        <div class="label pt-1">
                            <span class="label-text-alt text-base-content/40"
                                  x-text="$store.global.t('hint_upstream_protocol') || '发给上游时的协议，系统自动选目标协议的渠道'"></span>
                        </div>
                    </div>

                    <!-- ── STATUS (Pro Mode only) ─────────────────── -->
                    <template x-if="$store.global.proMode">
                        <div class="form-control w-full">
                            <div class="label pb-1">
                                <span class="label-text font-semibold" x-text="$store.global.t('label_route_status')"></span>
                            </div>
                            <select x-model="routeForm.is_active" class="select select-bordered select-sm w-full">
                                <option value="true" x-text="$store.global.t('status_enabled_short')"></option>
                                <option value="false" x-text="$store.global.t('status_disabled_short')"></option>
                            </select>
                        </div>
                    </template>

                </div>

                <div class="modal-action mt-6">
                    <button class="btn" @click="routeModal.show = false" x-text="$store.global.t('cancel')"></button>
                    <button class="btn btn-secondary shadow-lg shadow-secondary/20"
                            @click="saveRoute()" x-text="$store.global.t('btn_save_simple')"></button>
                </div>
            </div>
            <div class="modal-backdrop" @click="routeModal.show = false"></div>
        </dialog>
    </div>
    `
};
