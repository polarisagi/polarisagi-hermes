export default {
    name: 'rulesComponent',
    setup() {
        return {
            // ── Pro Mode State ───────────────────────────────────────────────
            routeModal: { show: false, isEdit: false },
            sourceMode: 'tier',
            selectedTier: 'smart',
            routeForm: {
                id: 0,
                requested_model_id: '',
                target_user_model_id: 0,
                is_active: true
            },

            // ── Simple Mode State ────────────────────────────────────────────
            intentModal: { show: false },
            intentForm: {
                model_id: '',
                capability_tier: 'smart'
            },
            userIntents: [],

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

            async fetchUserIntents() {
                try {
                    const res = await fetch('/api/admin/intents');
                    const json = await res.json() || [];
                    this.userIntents = Array.isArray(json) ? json : [];
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

            // ─── Simple Mode: Intent Modal ────────────────────────────────────

            openIntentModal() {
                this.intentForm = { model_id: '', capability_tier: 'smart' };
                this.intentModal.show = true;
            },

            async saveIntent() {
                const gStore = Alpine.store('global');
                const modelId = (this.intentForm.model_id || '').trim();
                if (!modelId) {
                    gStore.showToast('模型名称不能为空', 'error');
                    return;
                }
                try {
                    const res = await fetch('/api/admin/intents', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({
                            model_id: modelId,
                            capability_tier: this.intentForm.capability_tier
                        })
                    });
                    if (res.ok) {
                        gStore.showToast('意图映射已保存');
                        this.intentModal.show = false;
                        this.fetchUserIntents();
                    } else {
                        const err = await res.text();
                        gStore.showToast('保存失败: ' + err, 'error');
                    }
                } catch (e) {
                    gStore.showToast('网络错误', 'error');
                }
            },

            async deleteIntent(modelId) {
                const gStore = Alpine.store('global');
                if (!confirm(`确定要删除 "${modelId}" 的意图映射吗？`)) return;
                try {
                    const res = await fetch(`/api/admin/intents?model=${encodeURIComponent(modelId)}`, { method: 'DELETE' });
                    if (res.ok) {
                        gStore.showToast('已删除');
                        this.fetchUserIntents();
                    } else {
                        gStore.showToast('删除失败', 'error');
                    }
                } catch (e) {
                    gStore.showToast('网络错误', 'error');
                }
            },

            // ─── Pro Mode: Route Modal ────────────────────────────────────────

            openRouteModal(route = null) {
                const models = Alpine.store('global').allModels || [];
                if (route) {
                    const rid = route.requested_model_id || '';
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

            setSourceMode(mode) {
                this.sourceMode = mode;
                if (mode === 'tier') {
                    this.routeForm.requested_model_id = this.selectedTier || 'smart';
                } else if (mode === 'wildcard') {
                    this.routeForm.requested_model_id = '*';
                } else {
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
                this.fetchUserIntents();
                this.$watch('$store.global.currentTab', (newTab) => {
                    if (newTab === 'rules') {
                        this.fetchRoutes();
                        this.fetchAllModels();
                        this.fetchUserIntents();
                    }
                });
            }
        };
    },

    template: `
    <div x-show="$store.global.currentTab === 'rules'" class="max-w-6xl mx-auto w-full">

        <!-- ══════════════════════════════════════════════
             极简模式：意图映射管理
             ══════════════════════════════════════════════ -->
        <template x-if="!$store.global.proMode">
            <div>
                <!-- Header -->
                <div class="flex justify-between items-center mb-6">
                    <div>
                        <h2 class="text-3xl font-bold">意图映射</h2>
                        <p class="text-base-content/60 text-sm mt-1">为自定义模型名配置能力梯队，系统内置 570+ 条主流模型自动生效无需配置</p>
                    </div>
                    <button @click="openIntentModal()" class="btn btn-secondary shadow-lg shadow-secondary/20">
                        <span class="text-lg">+</span>
                        <span>添加意图映射</span>
                    </button>
                </div>

                <!-- 系统说明 Banner -->
                <div class="alert mb-4 bg-emerald-500/10 border border-emerald-500/30 py-3">
                    <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5 shrink-0 text-emerald-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
                    <div class="text-sm">
                        <div class="font-semibold text-emerald-400 mb-1">极简模式无需配置任何转发规则</div>
                        <div class="text-base-content/60 text-xs leading-relaxed">
                            系统已内置 <span class="font-mono font-bold text-info">570+</span> 条意图映射（gpt-4o→smart、claude-haiku→fast、o3→reasoning 等），覆盖所有主流 AI 客户端。
                            只要在「渠道账号」中添加 DeepSeek 或 Gemini 渠道，网关就会自动按能力梯队路由，无需手动配置。
                        </div>
                    </div>
                </div>

                <!-- 三梯队说明卡片 -->
                <div class="grid grid-cols-3 gap-3 mb-6">
                    <div class="card bg-warning/5 border border-warning/20 p-4">
                        <div class="text-xl mb-1">🏆</div>
                        <div class="font-bold text-sm">旗舰型 <span class="font-mono text-xs text-warning">smart</span></div>
                        <div class="text-xs text-base-content/50 mt-1">gpt-4o、claude-sonnet、gemini-pro<br>→ deepseek-v4-flash、gemini-3.1-pro</div>
                    </div>
                    <div class="card bg-info/5 border border-info/20 p-4">
                        <div class="text-xl mb-1">⚡</div>
                        <div class="font-bold text-sm">极速型 <span class="font-mono text-xs text-info">fast</span></div>
                        <div class="text-xs text-base-content/50 mt-1">gpt-4o-mini、claude-haiku、gemini-flash<br>→ deepseek-v4-flash、gemini-3.1-flash</div>
                    </div>
                    <div class="card bg-secondary/5 border border-secondary/20 p-4">
                        <div class="text-xl mb-1">🧠</div>
                        <div class="font-bold text-sm">沉思型 <span class="font-mono text-xs text-secondary">reasoning</span></div>
                        <div class="text-xs text-base-content/50 mt-1">o1、o3、DeepSeek-R1<br>→ deepseek-v4-pro、gemini-thinking</div>
                    </div>
                </div>

                <!-- 降级保障说明 -->
                <div class="alert mb-5 bg-base-200/60 border border-base-300/40 py-2.5">
                    <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 shrink-0 opacity-50" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
                    <span class="text-xs text-base-content/60">
                        <b>Tier 降级熔断保障：</b>当目标梯队无可用节点时（如后端无 fast 模型），系统自动降级至 smart 梯队兜底，保证服务不中断。
                        <b class="ml-1">同时支持 Claude Code（/v1/messages）和 Codex（/v1/chat/completions）双协议接入。</b>
                    </span>
                </div>

                <!-- 用户自定义映射表 -->
                <div class="card bg-base-100 shadow overflow-x-auto">
                    <div class="px-5 py-4 border-b border-base-300/40 flex items-center justify-between">
                        <div>
                            <h3 class="font-semibold">我的自定义意图映射</h3>
                            <p class="text-xs text-base-content/50 mt-0.5">仅当使用内部自定义模型名时才需要在此添加</p>
                        </div>
                        <span class="badge badge-ghost badge-sm" x-text="userIntents.length + ' 条'"></span>
                    </div>
                    <table class="table table-zebra w-full">
                        <thead>
                            <tr>
                                <th>客户端发来的模型名</th>
                                <th>映射到能力梯队</th>
                                <th class="text-right">操作</th>
                            </tr>
                        </thead>
                        <tbody>
                            <template x-for="intent in userIntents" :key="intent.model_id">
                                <tr>
                                    <td>
                                        <span class="font-mono text-info text-sm font-semibold" x-text="intent.model_id"></span>
                                    </td>
                                    <td>
                                        <span class="badge badge-sm font-mono"
                                              :class="getTierBadgeClass(intent.capability_tier)"
                                              x-text="getTierLabel(intent.capability_tier)"></span>
                                    </td>
                                    <td class="text-right">
                                        <button @click="deleteIntent(intent.model_id)"
                                                class="btn btn-ghost btn-xs text-error">删除</button>
                                    </td>
                                </tr>
                            </template>
                            <template x-if="userIntents.length === 0">
                                <tr>
                                    <td colspan="3" class="text-center py-10 text-base-content/40">
                                        <div class="text-3xl mb-2">✅</div>
                                        <div class="font-medium">无需自定义意图映射</div>
                                        <div class="text-xs mt-1">系统内置 570+ 条映射已覆盖所有主流 AI 客户端，正常工作中</div>
                                    </td>
                                </tr>
                            </template>
                        </tbody>
                    </table>
                </div>

                <!-- Intent Add Modal -->
                <dialog class="modal" :class="intentModal.show ? 'modal-open' : ''">
                    <div class="modal-box w-11/12 max-w-md">
                        <button class="btn btn-sm btn-circle btn-ghost absolute right-2 top-2"
                                @click="intentModal.show = false">✕</button>

                        <h3 class="font-bold text-lg mb-1">添加意图映射</h3>
                        <p class="text-xs text-base-content/50 mb-6">将您的内部自定义模型名映射到对应的能力梯队</p>

                        <div class="space-y-5">
                            <!-- 模型名输入 -->
                            <div class="form-control">
                                <div class="label pb-1">
                                    <span class="label-text font-semibold">客户端发来的模型名 *</span>
                                </div>
                                <input x-model="intentForm.model_id"
                                       type="text"
                                       class="input input-bordered w-full font-mono text-info"
                                       placeholder="e.g. my-assistant, internal-coder" />
                                <div class="label pt-1">
                                    <span class="label-text-alt text-base-content/40">客户端在请求中携带的 model 字段值</span>
                                </div>
                            </div>

                            <!-- 梯队选择 -->
                            <div class="form-control">
                                <div class="label pb-2">
                                    <span class="label-text font-semibold">映射到能力梯队 *</span>
                                </div>
                                <div class="grid grid-cols-3 gap-2">
                                    <button type="button"
                                            @click="intentForm.capability_tier = 'smart'"
                                            :class="intentForm.capability_tier === 'smart'
                                                ? 'ring-2 ring-warning ring-offset-2 ring-offset-base-100 bg-warning/10 border-warning'
                                                : 'border-base-300 hover:border-warning/50'"
                                            class="btn btn-outline border-2 flex-col h-auto py-3 gap-1 transition-all">
                                        <span class="text-xl">🏆</span>
                                        <span class="font-bold text-xs">旗舰型</span>
                                        <span class="font-mono text-xs opacity-60">smart</span>
                                    </button>
                                    <button type="button"
                                            @click="intentForm.capability_tier = 'fast'"
                                            :class="intentForm.capability_tier === 'fast'
                                                ? 'ring-2 ring-info ring-offset-2 ring-offset-base-100 bg-info/10 border-info'
                                                : 'border-base-300 hover:border-info/50'"
                                            class="btn btn-outline border-2 flex-col h-auto py-3 gap-1 transition-all">
                                        <span class="text-xl">⚡</span>
                                        <span class="font-bold text-xs">极速型</span>
                                        <span class="font-mono text-xs opacity-60">fast</span>
                                    </button>
                                    <button type="button"
                                            @click="intentForm.capability_tier = 'reasoning'"
                                            :class="intentForm.capability_tier === 'reasoning'
                                                ? 'ring-2 ring-secondary ring-offset-2 ring-offset-base-100 bg-secondary/10 border-secondary'
                                                : 'border-base-300 hover:border-secondary/50'"
                                            class="btn btn-outline border-2 flex-col h-auto py-3 gap-1 transition-all">
                                        <span class="text-xl">🧠</span>
                                        <span class="font-bold text-xs">沉思型</span>
                                        <span class="font-mono text-xs opacity-60">reasoning</span>
                                    </button>
                                </div>
                            </div>
                        </div>

                        <div class="modal-action mt-6">
                            <button class="btn" @click="intentModal.show = false">取消</button>
                            <button class="btn btn-secondary shadow-lg shadow-secondary/20"
                                    @click="saveIntent()">保存映射</button>
                        </div>
                    </div>
                    <div class="modal-backdrop" @click="intentModal.show = false"></div>
                </dialog>
            </div>
        </template>

        <!-- ══════════════════════════════════════════════
             Pro 模式：精确路由规则（保持原有功能）
             ══════════════════════════════════════════════ -->
        <template x-if="$store.global.proMode">
            <div>
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

                <!-- Pro Mode Banner -->
                <div class="alert mb-4 bg-primary/5 border border-primary/20 py-2.5">
                    <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 shrink-0 text-primary" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4M7.835 4.697a3.42 3.42 0 001.946-.806 3.42 3.42 0 014.438 0 3.42 3.42 0 001.946.806 3.42 3.42 0 013.138 3.138 3.42 3.42 0 00.806 1.946 3.42 3.42 0 010 4.438 3.42 3.42 0 00-.806 1.946 3.42 3.42 0 01-3.138 3.138 3.42 3.42 0 00-1.946.806 3.42 3.42 0 01-4.438 0 3.42 3.42 0 00-1.946-.806 3.42 3.42 0 01-3.138-3.138 3.42 3.42 0 00-.806-1.946 3.42 3.42 0 010-4.438 3.42 3.42 0 00.806-1.946 3.42 3.42 0 013.138-3.138z"/></svg>
                    <span class="text-xs text-primary/80" x-text="$store.global.t('routes_pro_hint') || 'Pro 模式：精确配置模型 ID → 目标模型的 1:1 强制映射，优先级高于智能推断。支持精确匹配、正则表达式和通配符 * 兜底。'"></span>
                </div>

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
                                        <template x-if="isSourceTier(route.requested_model_id)">
                                            <div class="flex items-center gap-2">
                                                <span class="badge badge-sm font-mono font-semibold"
                                                      :class="getTierBadgeClass(route.requested_model_id)"
                                                      x-text="getTierLabel(route.requested_model_id)"></span>
                                            </div>
                                        </template>
                                        <template x-if="route.requested_model_id === '*'">
                                            <div class="flex items-center gap-2">
                                                <span class="badge badge-ghost badge-sm font-mono">✸ * 全部</span>
                                            </div>
                                        </template>
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
                     Route Editor Modal (Pro Mode)
                     ══════════════════════════════════════════════ -->
                <dialog class="modal" :class="routeModal.show ? 'modal-open' : ''">
                    <div class="modal-box w-11/12 max-w-2xl">
                        <button class="btn btn-sm btn-circle btn-ghost absolute right-2 top-2"
                                @click="routeModal.show = false">✕</button>

                        <h3 class="font-bold text-lg mb-1"
                            x-text="routeModal.isEdit ? $store.global.t('edit_route') : $store.global.t('add_new_route')"></h3>
                        <p class="text-xs text-base-content/50 mb-6"
                           x-text="$store.global.t('route_modal_pro_desc') || '精确配置：源模型 ID → 目标模型 ID 的强制映射规则'"></p>

                        <div class="space-y-6">

                            <!-- SOURCE: Pro Mode -->
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

                            <!-- TARGET MODEL -->
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
                            </div>

                            <!-- STATUS -->
                            <div class="form-control w-full">
                                <div class="label pb-1">
                                    <span class="label-text font-semibold" x-text="$store.global.t('label_route_status')"></span>
                                </div>
                                <select x-model="routeForm.is_active" class="select select-bordered select-sm w-full">
                                    <option value="true" x-text="$store.global.t('status_enabled_short')"></option>
                                    <option value="false" x-text="$store.global.t('status_disabled_short')"></option>
                                </select>
                            </div>

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
        </template>

    </div>
    `
};
