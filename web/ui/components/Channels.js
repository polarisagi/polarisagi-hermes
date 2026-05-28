export default {
    name: 'channelsComponent',
    setup() {
        return {
            nodeModal: { show: false, isEdit: false },
            modelsModal: { show: false, nodeId: 0, nodeName: '', nodeProvider: '' },
            channelModels: [],
            addModelModal: { show: false },
            addModelForm: { model_id: '', capability_tier: 'smart' },
            sysProviders: [],
            sysEndpoints: [],
            availableEndpoints: [],
            selectedEndpoint: null,
            nodeForm: {
                id: 0, provider: 'openai', name: '', credentials: '', project_id: '', location: 'global', base_url: '',
                priority: 10, limit_percent: 90.0, balance: 0.0, min_request_interval_sec: 0, concurrency: 0,
                valid_from: '', valid_to: '', status: 1, enable_claude: false
            },
            
            toDatetimeLocal(dt) {
                if (!dt) return '';
                dt = dt.trim();
                dt = dt.replace(/Z$/, '').replace(/[+-]\\d{2}:\\d{2}$/, '');
                if (dt.length === 10) return dt + 'T00:00:00';
                return dt.replace(' ', 'T');
            },
            fromDatetimeLocal(dt) { return dt ? dt.trim().replace('T', ' ') : ''; },
            todayPrefix() {
                const d = new Date();
                const pad = n => String(n).padStart(2, '0');
                return `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}`;
            },

            usagePercent(node) {
                if (!node.balance || node.balance <= 0) return 0;
                return ((node.used_amount || 0) / node.balance) * 100;
            },

            async fetchNodes() {
                if (Alpine.store('global').currentTab !== 'channels' && Alpine.store('global').currentTab !== 'rules') return;
                try {
                    const res = await fetch('/api/admin/nodes');
                    let nodes = await res.json() || [];
                    nodes = nodes.map(n => {
                        // find endpoint
                        const ep = this.sysEndpoints.find(e => e.provider_id === n.provider_id);
                        n.provider = n.provider_id;
                        n.concurrency = n.concurrency_limit || 0;
                        n.min_request_interval_sec = n.min_interval_sec || 0;
                        return n;
                    });
                    Alpine.store('global').nodes = nodes;
                } catch (e) { console.error(e) }
            },

            async fetchAllModels() {
                try {
                    const res = await fetch('/api/admin/models');
                    const json = await res.json() || [];
                    Alpine.store('global').allModels = Array.isArray(json) ? json : [];
                } catch (e) { console.error(e); }
            },

            async fetchSysProviders() {
                try {
                    const res = await fetch('/api/admin/sys_providers');
                    const data = await res.json();
                    if (data && data.providers) {
                        this.sysProviders = data.providers;
                        this.sysEndpoints = data.endpoints;
                    }
                } catch (e) { console.error("Failed to fetch sys_providers", e); }
            },

            // ── Model Sub-Panel Methods ────────────────────────────────────────

            getModelsForChannel(nodeId) {
                const all = Alpine.store('global').allModels || [];
                return all.filter(m => m.user_provider_id === nodeId);
            },

            getTierBadgeClass(tier) {
                const map = { smart: 'badge-warning', fast: 'badge-info', reasoning: 'badge-secondary' };
                return map[tier] || 'badge-ghost';
            },

            openModelsModal(node) {
                this.modelsModal = {
                    show: true,
                    nodeId: node.id,
                    nodeName: node.name,
                    nodeProvider: node.provider
                };
                this.channelModels = this.getModelsForChannel(node.id);
            },

            async changeModelTier(modelId, newTier) {
                const gStore = Alpine.store('global');
                try {
                    const res = await fetch('/api/admin/models', {
                        method: 'PUT',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ id: modelId, capability_tier: newTier })
                    });
                    if (res.ok) {
                        await this.fetchAllModels();
                        this.channelModels = this.getModelsForChannel(this.modelsModal.nodeId);
                        gStore.showToast('梯队已更新');
                    } else {
                        gStore.showToast('更新失败', 'error');
                    }
                } catch (e) { gStore.showToast('网络错误', 'error'); }
            },

            async removeModel(modelId) {
                const gStore = Alpine.store('global');
                if (!confirm('确定要从该渠道中移除此模型吗？')) return;
                try {
                    const res = await fetch(`/api/admin/models?id=${modelId}`, { method: 'DELETE' });
                    if (res.ok) {
                        await this.fetchAllModels();
                        this.channelModels = this.getModelsForChannel(this.modelsModal.nodeId);
                        gStore.showToast('已移除');
                    } else {
                        gStore.showToast('移除失败', 'error');
                    }
                } catch (e) { gStore.showToast('网络错误', 'error'); }
            },

            async addModelToChannel() {
                const gStore = Alpine.store('global');
                const modelId = (this.addModelForm.model_id || '').trim();
                if (!modelId) {
                    gStore.showToast('模型名称不能为空', 'error');
                    return;
                }
                try {
                    const res = await fetch('/api/admin/models', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({
                            user_provider_id: this.modelsModal.nodeId,
                            model_id: modelId,
                            capability_tier: this.addModelForm.capability_tier,
                            display_name: modelId
                        })
                    });
                    if (res.ok) {
                        this.addModelModal.show = false;
                        this.addModelForm = { model_id: '', capability_tier: 'smart' };
                        await this.fetchAllModels();
                        this.channelModels = this.getModelsForChannel(this.modelsModal.nodeId);
                        gStore.showToast('模型已添加');
                    } else {
                        const err = await res.text();
                        gStore.showToast('添加失败: ' + err, 'error');
                    }
                } catch (e) { gStore.showToast('网络错误', 'error'); }
            },

            openNodeModal(node = null) {
                if (node) {
                    node.provider = node.provider_id;
                    const ep = this.sysEndpoints.find(e => e.provider_id === node.provider_id);
                    
                    const origCreds = node.auth_credentials || {};

                    this.nodeForm = {
                        ...node,
                        
                        credentials: '',
                        project_id: origCreds.project_id || '',
                        location: origCreds.region || 'global',
                        limit_percent: node.limit_percent !== undefined ? node.limit_percent : 90.0,
                        valid_from: this.toDatetimeLocal(node.valid_from),
                        valid_to: this.toDatetimeLocal(node.valid_to),
                    };
                    this.nodeModal = { show: true, isEdit: true };
                } else {
                    const today = this.todayPrefix();
                    this.nodeForm = {
                        id: 0, provider: 'openai', name: '', credentials: '', project_id: '', location: 'global', base_url: '',
                        priority: 10, limit_percent: 90.0, balance: 0.0, min_request_interval_sec: 0, concurrency: 0,
                        valid_from: `${today}T00:00:00`, valid_to: `2099-12-31T23:59:59`, status: 1, enable_claude: false
                    };
                    this.nodeModal = { show: true, isEdit: false };
                }
            },

            async saveNode() {
                const form = this.nodeForm;
                const gStore = Alpine.store('global');
                if (!form.name || (!this.nodeModal.isEdit && !form.credentials && form.provider !== 'ollama')) {
                    gStore.showToast(gStore.t('err_empty_node'), 'error');
                    return;
                }
                if (form.provider === 'google' && !form.project_id) {
                    gStore.showToast(gStore.t('err_gcp_project'), 'error');
                    return;
                }
                if (form.priority < 0 || form.balance < 0 || form.limit_percent < 0) {
                    gStore.showToast(gStore.t('err_negative_numbers'), 'error');
                    return;
                }
                if (form.limit_percent > 100) {
                    gStore.showToast(gStore.t('err_limit_exceed'), 'error');
                    return;
                }
                if (form.concurrency < 0 || form.concurrency > 1000) {
                    gStore.showToast('并发限制必须在 0 到 1000 之间', 'error');
                    return;
                }

                try {
                    const method = this.nodeModal.isEdit ? 'PUT' : 'POST';
                    
                    // Map form fields to backend expectations
                    let authCreds = {};
                    if (this.nodeModal.isEdit && !form.credentials) {
                        authCreds = { ...(form.auth_credentials || {}) };
                    } else if (this.selectedEndpoint) {
                        if (this.selectedEndpoint.auth_type === 'adc') {
                            try {
                                authCreds = JSON.parse(form.credentials);
                            } catch(e) {
                                authCreds.adc_json = form.credentials;
                            }
                        } else if (this.selectedEndpoint.auth_type !== 'none') {
                            authCreds.api_key = form.credentials;
                        }
                    }
                    if (this.selectedEndpoint) {
                        if (this.selectedEndpoint.required_credential_fields.includes('project_id')) authCreds.project_id = form.project_id;
                        if (this.selectedEndpoint.required_credential_fields.includes('region')) authCreds.region = form.location;
                    }
                    
                    const payload = {
                        ...form,
                        provider_id: form.provider,
                        auth_credentials: authCreds,
                        concurrency_limit: form.concurrency,
                        min_interval_sec: form.min_request_interval_sec,
                        valid_from: this.fromDatetimeLocal(form.valid_from),
                        valid_to: this.fromDatetimeLocal(form.valid_to),
                        enable_claude: form.enable_claude,
                    };
                    const res = await fetch('/api/admin/nodes', {
                        method,
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify(payload)
                    });
                    if (res.ok) {
                        gStore.showToast(this.nodeModal.isEdit ? gStore.t('node_updated') : gStore.t('node_added'));
                        this.nodeModal.show = false;
                        this.fetchNodes();
                    } else {
                        const err = await res.text();
                        gStore.showToast(gStore.t('save_failed') + ': ' + err, 'error');
                    }
                } catch(e) {
                    gStore.showToast(gStore.t('network_error'), 'error');
                }
            },

            async deleteNode(id) {
                const gStore = Alpine.store('global');
                if(!confirm(gStore.lang === 'zh' ? '确定要删除这个节点吗？此操作不可恢复。' : 'Are you sure you want to delete this node? This action cannot be undone.')) return;
                try {
                    const res = await fetch(`/api/admin/nodes?id=${id}`, { method: 'DELETE' });
                    if (res.ok) {
                        gStore.showToast(gStore.t('node_deleted'));
                        this.fetchNodes();
                    } else {
                        gStore.showToast(gStore.t('delete_failed'), 'error');
                    }
                } catch(e) {
                    gStore.showToast(gStore.t('network_error'), 'error');
                }
            },

            startGoogleAuth() {
                const gStore = Alpine.store('global');
                const isLocal = window.location.hostname === '127.0.0.1' || window.location.hostname === 'localhost';
                if (!isLocal) {
                    alert(gStore.t("oauth_alert"));
                    return;
                }
                
                const receiveMessage = (event) => {
                    if (event.data && event.data.type === 'google_adc_auth' && event.data.data) {
                        this.nodeForm.credentials = event.data.data;
                        gStore.showToast(gStore.t('adc_filled'));
                        window.removeEventListener('message', receiveMessage);
                    }
                };
                window.addEventListener('message', receiveMessage, false);

                const width = 600;
                const height = 700;
                const left = Math.max(0, (window.innerWidth - width) / 2 + window.screenX);
                const top = Math.max(0, (window.innerHeight - height) / 2 + window.screenY);
                window.open('/api/admin/oauth/google/start', 'GoogleAuth', `width=${width},height=${height},top=${top},left=${left}`);
            },

            init() {
                this.fetchSysProviders();
                this.fetchNodes();
                this.fetchAllModels();

                
                // Function to update computed auth mode state
                const updateAuthModes = () => {
                    this.availableEndpoints = this.sysEndpoints.filter(m => m.provider_id === this.nodeForm.provider);
                    if (this.availableEndpoints.length > 0) {
                        this.selectedEndpoint = this.availableEndpoints[0];
                    } else {
                        this.selectedEndpoint = null;
                    }
                };

                this.$watch('nodeForm.provider', (newVal) => {
                    if (!this.nodeModal.isEdit) {
                        updateAuthModes();
                    }
                    if (!this.nodeModal.isEdit && newVal === 'vertex') {
                        this.nodeForm.concurrency = 1;
                    } else if (!this.nodeModal.isEdit && newVal !== 'vertex') {
                        this.nodeForm.concurrency = 0;
                    }
                    if (!this.nodeModal.isEdit) {
                        const defaultURLs = {
                            'ollama': 'http://127.0.0.1:11434/v1',
                            'deepseek': 'https://api.deepseek.com/v1',
                            'siliconflow': 'https://api.siliconflow.cn/v1',
                            'grok': 'https://api.x.ai/v1',
                            'openrouter': 'https://openrouter.ai/api/v1',
                        };
                        if (defaultURLs[newVal]) {
                            this.nodeForm.base_url = defaultURLs[newVal];
                            if (newVal === 'ollama') this.nodeForm.credentials = '';
                        } else if (Object.values(defaultURLs).includes(this.nodeForm.base_url)) {
                            this.nodeForm.base_url = '';
                        }
                    }
                });


                
                // Also trigger initial calculation when modal opens
                this.$watch('nodeModal.show', (newVal) => {
                    if (newVal) {
                        updateAuthModes();
                    }
                });
                
                this.$watch('$store.global.currentTab', (newTab) => {
                    if (newTab === 'channels') {
                        this.fetchNodes();
                        this.fetchAllModels();
                    }
                });
            }
        };
    },
    template: `
        <div x-show="$store.global.currentTab === 'channels'" class="max-w-6xl mx-auto w-full">
            <div class="flex justify-between items-center mb-6">
                <div>
                    <h2 class="text-3xl font-bold" x-text="$store.global.t('tab_nodes_title')"></h2>
                    <p class="text-base-content/60 text-sm mt-2" x-text="$store.global.t('channels_subtitle')"></p>
                </div>
                <button @click="openNodeModal()" class="btn btn-primary shadow-lg shadow-primary/20">
                    <span class="text-lg">+</span> <span x-text="$store.global.t('btn_add_new_node')"></span>
                </button>
            </div>
            
            <div class="card bg-base-100 shadow overflow-x-auto">
                <table class="table table-zebra w-full">
                    <thead>
                        <tr>
                            <th x-text="$store.global.t('table_platform')"></th>
                            <th x-text="$store.global.t('node_name')"></th>
                            <th class="text-center">Pri / Concurrency</th>
                            <th x-text="$store.global.t('table_limit_usage')"></th>
                            <th x-text="$store.global.t('valid_range')"></th>
                            <th class="text-center">模型</th>
                            <th class="text-center w-20" x-text="$store.global.t('table_status')"></th>
                            <th class="text-right w-24" x-text="$store.global.t('actions')"></th>
                        </tr>
                    </thead>
                    <tbody>
                        <template x-for="node in $store.global.nodes" :key="node.id">
                            <tr>
                                <td>
                                    <span :class="$store.global.protocolBadge(node.provider)"
                                        class="badge badge-sm font-bold uppercase" x-text="$store.global.protocolLabel(node.provider)"></span>
                                </td>
                                <td class="font-medium" x-text="node.name"></td>
                                <td class="text-center text-xs">
                                    <div class="font-bold">Pri: <span x-text="node.priority"></span></div>
                                    <div class="text-base-content/50">Con: <span x-text="node.concurrency === 0 ? '∞' : node.concurrency"></span></div>
                                </td>
                                <td>
                                    <template x-if="node.balance > 0">
                                        <div class="space-y-1">
                                            <div class="flex items-center justify-between text-[10px]">
                                                <span class="text-base-content/50">$\x3Cspan x-text="$store.global.formatNum(node.used_amount || 0)">\x3C/span> / $\x3Cspan x-text="$store.global.formatNum(node.balance)">\x3C/span></span>
                                                <span :class="usagePercent(node) >= node.limit_percent ? 'text-error' : 'text-base-content/50'" x-text="usagePercent(node).toFixed(1) + '%'"></span>
                                            </div>
                                            <progress class="progress w-full" :class="usagePercent(node) >= node.limit_percent ? 'progress-error' : 'progress-success'" :value="Math.min(usagePercent(node), 100)" max="100"></progress>
                                        </div>
                                    </template>
                                    <template x-if="!(node.balance > 0)">
                                        <span class="text-base-content/50 text-xs" x-text="$store.global.t('no_limit_text')"></span>
                                    </template>
                                </td>
                                <td class="text-xs text-base-content/60">
                                    <template x-if="node.valid_from && node.valid_to">
                                        <div>
                                            <span x-text="$store.global.formatShortDate(node.valid_from)"></span><br><span class="text-base-content/30">~</span> <span x-text="$store.global.formatShortDate(node.valid_to)"></span>
                                        </div>
                                    </template>
                                    <template x-if="!(node.valid_from && node.valid_to)">
                                        <span class="text-base-content/30">-</span>
                                    </template>
                                </td>
                                <td class="text-center">
                                    <button @click="openModelsModal(node)"
                                            class="btn btn-ghost btn-xs font-mono gap-1">
                                        <span>⚙</span>
                                        <span class="text-info" x-text="getModelsForChannel(node.id).length"></span>
                                    </button>
                                </td>
                                <td class="text-center">
                                    <template x-if="node.status === 1"><span class="badge badge-success badge-sm" x-text="$store.global.t('status_enabled_short')"></span></template>
                                    <template x-if="node.status === 0"><span class="badge badge-ghost badge-sm" x-text="$store.global.t('status_disabled_short')"></span></template>
                                    <template x-if="node.status === -1"><span class="badge badge-error badge-sm" x-text="$store.global.t('status_exhausted_short')"></span></template>
                                </td>
                                <td class="text-right space-x-2">
                                    <button @click="openNodeModal(node)" class="btn btn-ghost btn-xs text-info" x-text="$store.global.t('edit')"></button>
                                    <button @click="deleteNode(node.id)" class="btn btn-ghost btn-xs text-error" x-text="$store.global.t('delete')"></button>
                                </td>
                            </tr>
                        </template>
                        <template x-if="$store.global.nodes.length === 0">
                            <tr>
                                <td colspan="8" class="text-center py-8 text-base-content/50" x-text="$store.global.t('no_nodes')"></td>
                            </tr>
                        </template>
                    </tbody>
                </table>
            </div>

            <!-- Node Editor Modal -->
            <dialog class="modal" :class="nodeModal.show ? 'modal-open' : ''">
                <div class="modal-box w-11/12 max-w-3xl">
                    <button class="btn btn-sm btn-circle btn-ghost absolute right-2 top-2" @click="nodeModal.show = false">✕</button>
                    <h3 class="font-bold text-lg mb-6" x-text="nodeModal.isEdit ? $store.global.t('edit_node') : $store.global.t('add_new_node')"></h3>
                    
                    <div class="space-y-6">
                        <!-- 区块 1: 基本信息 -->
                        <div class="bg-base-200 p-4 rounded-xl space-y-4 border border-base-300">
                            <h4 class="text-xs font-bold text-base-content/50 uppercase tracking-wider" x-text="$store.global.t('section_basic')"></h4>
                            <div class="grid grid-cols-2 gap-4">

                                <label class="form-control w-full">
                                    <div class="label"><span class="label-text font-medium">大模型厂商 <span class="text-error">*</span></span></div>
                                    <select name="nodeForm_provider" x-model="nodeForm.provider" class="select select-bordered select-sm w-full">
                                        <template x-for="p in sysProviders" :key="p.provider_id">
                                            <option :value="p.provider_id" x-text="p.provider_name"></option>
                                        </template>
                                    </select>
                                </label>

                                <template x-if="nodeForm.provider === 'gemini_enterprise_agent_platform' && !nodeModal.isEdit">
                                    <div class="form-control w-full justify-center">
                                        <label class="label cursor-pointer pb-0 mt-2">
                                            <span class="label-text font-medium">开启 Claude 模型支持</span> 
                                            <input name="nodeForm_enable_claude" type="checkbox" x-model="nodeForm.enable_claude" class="toggle toggle-primary toggle-sm" />
                                        </label>
                                        <div class="label pt-1"><span class="label-text-alt text-base-content/50">若关闭，将不导入 Claude 模型</span></div>
                                    </div>
                                </template>
                            </div>
                            <div class="grid grid-cols-1 gap-4">
                                <label class="form-control w-full">
                                    <div class="label"><span class="label-text font-medium"><span x-text="$store.global.t('node_name_req')"></span> <span class="text-error">*</span></span></div>
                                    <input name="nodeForm_name" x-model="nodeForm.name" type="text" :placeholder="$store.global.t('placeholder_node_name')" class="input input-bordered input-sm w-full">
                                </label>
                            </div>
                            

                            
                            <label class="form-control w-full">
                                <div class="label">
                                    <span class="label-text font-medium">
                                        <template x-if="selectedEndpoint && selectedEndpoint.auth_type === 'adc'"><span>ADC JSON</span></template>
                                        <template x-if="!selectedEndpoint || selectedEndpoint.auth_type !== 'adc'"><span>API Key</span></template>
                                        
                                        <template x-if="selectedEndpoint && selectedEndpoint.auth_type !== 'none'"><span class="text-error">*</span></template>
                                        
                                        <!-- 动态提示词 -->
                                        <template x-if="selectedEndpoint && selectedEndpoint.auth_type === 'adc'"><span class="text-base-content/50 text-xs ml-1 font-normal" x-text="$store.global.t('hint_adc_paste')"></span></template>
                                        <template x-if="selectedEndpoint && selectedEndpoint.auth_type === 'header'"><span class="text-base-content/50 text-xs ml-1 font-normal" x-text="'(放在 ' + selectedEndpoint.auth_header + ' 请求头中)'"></span></template>
                                        <template x-if="selectedEndpoint && selectedEndpoint.auth_type === 'bearer'"><span class="text-base-content/50 text-xs ml-1 font-normal" x-text="$store.global.t('hint_sk_bearer')"></span></template>
                                        <template x-if="selectedEndpoint && selectedEndpoint.auth_type === 'none'"><span class="text-base-content/50 text-xs ml-1 font-normal">通常无需验证，留空即可</span></template>
                                    </span>
                                    <template x-if="selectedEndpoint && selectedEndpoint.auth_type === 'adc'">
                                        <button @click="startGoogleAuth" class="btn btn-xs btn-outline btn-info">🔑 <span x-text="$store.global.t('btn_oauth_auto')"></span></button>
                                    </template>
                                </div>
                                
                                <template x-if="selectedEndpoint && selectedEndpoint.auth_type === 'adc'">
                                    <textarea name="nodeForm_credentials" x-model="nodeForm.credentials" rows="3" :placeholder="nodeModal.isEdit ? $store.global.t('placeholder_adc_edit') : $store.global.t('placeholder_adc_new')" class="textarea textarea-bordered font-mono text-xs w-full"></textarea>
                                </template>
                                <template x-if="!selectedEndpoint || selectedEndpoint.auth_type !== 'adc'">
                                    <input name="nodeForm_credentials" x-model="nodeForm.credentials" type="password" :placeholder="nodeModal.isEdit ? $store.global.t('placeholder_key_edit') : $store.global.t('placeholder_key_new')" class="input input-bordered input-sm w-full">
                                </template>
                            </label>
                            
                            <template x-if="$store.global.proMode">
                                <div class="grid grid-cols-2 gap-4">
                                    <label class="form-control w-full">
                                        <div class="label"><span class="label-text" x-text="$store.global.t('priority')"></span></div>
                                        <input name="nodeForm_priority" x-model.number="nodeForm.priority" type="number" min="0" class="input input-bordered input-sm w-full">
                                        <div class="label"><span class="label-text-alt text-base-content/50" x-text="$store.global.t('priority_hint')"></span></div>
                                    </label>
                                    <label class="form-control w-full">
                                        <div class="label"><span class="label-text" x-text="$store.global.t('min_interval_label')"></span></div>
                                        <input name="nodeForm_min_request_interval_sec" x-model.number="nodeForm.min_request_interval_sec" type="number" min="0" class="input input-bordered input-sm w-full">
                                        <div class="label"><span class="label-text-alt text-base-content/50" x-text="$store.global.t('min_interval_hint')"></span></div>
                                    </label>
                                </div>
                            </template>
                            <template x-if="$store.global.proMode">
                                <div class="grid grid-cols-2 gap-4">
                                    <label class="form-control w-full">
                                        <div class="label"><span class="label-text" x-text="$store.global.t('status')"></span></div>
                                        <select name="nodeForm_status" x-model.number="nodeForm.status" class="select select-bordered select-sm w-full">
                                            <option value="1" x-text="$store.global.t('status_option_enable')"></option>
                                            <option value="0" x-text="$store.global.t('status_option_disable')"></option>
                                            <option value="-1" x-text="$store.global.t('status_option_exhaust')"></option>
                                        </select>
                                    </label>
                                    <label class="form-control w-full">
                                        <div class="label"><span class="label-text">Concurrency (并发限制)</span></div>
                                        <input name="nodeForm_concurrency" x-model.number="nodeForm.concurrency" type="number" min="0" max="1000" class="input input-bordered input-sm w-full">
                                        <div class="label"><span class="label-text-alt text-base-content/50">0 为无限制，上限 1000</span></div>
                                    </label>
                                </div>
                            </template>
                        </div>

                        <!-- 区块 2: 供应商配置 -->
                        <div class="bg-base-200 p-4 rounded-xl space-y-4 border border-base-300">
                            <h4 class="text-xs font-bold text-base-content/50 uppercase tracking-wider" x-text="$store.global.t('section_provider')"></h4>
                            <template x-if="selectedEndpoint && (selectedEndpoint.required_credential_fields.includes('project_id') || selectedEndpoint.required_credential_fields.includes('region'))">
                                <div class="grid grid-cols-2 gap-4">
                                    <template x-if="selectedEndpoint.required_credential_fields.includes('project_id')">
                                        <label class="form-control w-full">
                                            <div class="label"><span class="label-text font-medium"><span x-text="$store.global.t('gcp_project_id')"></span> <span class="text-error">*</span></span></div>
                                            <input name="nodeForm_project_id" x-model="nodeForm.project_id" type="text" placeholder="your-gcp-project-id" class="input input-bordered input-sm w-full">
                                        </label>
                                    </template>
                                    <template x-if="selectedEndpoint.required_credential_fields.includes('region')">
                                        <label class="form-control w-full">
                                            <div class="label"><span class="label-text" x-text="$store.global.t('gcp_location')"></span></div>
                                            <input name="nodeForm_location" x-model="nodeForm.location" type="text" placeholder="global" class="input input-bordered input-sm w-full">
                                            <div class="label"><span class="label-text-alt text-base-content/50" x-text="$store.global.t('hint_location')"></span></div>
                                        </label>
                                    </template>
                                </div>
                            </template>
                            <template x-if="$store.global.proMode || nodeForm.provider === 'ollama' || nodeForm.provider === 'deepseek' || nodeForm.provider === 'siliconflow' || nodeForm.provider === 'grok' || nodeForm.provider === 'openrouter'">
                                <label class="form-control w-full">
                                    <div class="label"><span class="label-text" x-text="$store.global.t('base_url_optional')"></span></div>
                                    <input name="nodeForm_base_url" x-model="nodeForm.base_url" type="text" :placeholder="$store.global.t('placeholder_baseurl')" class="input input-bordered input-sm w-full">
                                    <div class="label"><span class="label-text-alt text-base-content/50" x-text="$store.global.t('hint_custom_endpoint')"></span></div>
                                </label>
                            </template>
                        </div>

                        <!-- 区块 3: 计费与有效期 -->
                        <template x-if="$store.global.proMode">
                            <div class="bg-base-200 p-4 rounded-xl space-y-4 border border-base-300">
                                <h4 class="text-xs font-bold text-base-content/50 uppercase tracking-wider" x-text="$store.global.t('section_billing_validity')"></h4>
                                <div class="grid grid-cols-2 gap-4">
                                    <label class="form-control w-full">
                                        <div class="label"><span class="label-text" x-text="$store.global.t('label_total_balance')"></span></div>
                                        <input name="nodeForm_balance" x-model.number="nodeForm.balance" type="number" min="0" step="0.01" placeholder="0.00" class="input input-bordered input-sm w-full">
                                        <div class="label"><span class="label-text-alt text-base-content/50" x-text="$store.global.t('hint_unlimited')"></span></div>
                                    </label>
                                    <label class="form-control w-full">
                                        <div class="label"><span class="label-text" x-text="$store.global.t('label_limit_percent')"></span></div>
                                        <input name="nodeForm_limit_percent" x-model.number="nodeForm.limit_percent" type="number" min="0" max="100" step="0.1" class="input input-bordered input-sm w-full">
                                        <div class="label"><span class="label-text-alt text-base-content/50" x-text="$store.global.t('hint_limit_percent')"></span></div>
                                    </label>
                                    <label class="form-control w-full">
                                        <div class="label"><span class="label-text" x-text="$store.global.t('label_valid_from')"></span></div>
                                        <input name="nodeForm_valid_from" x-model="nodeForm.valid_from" type="datetime-local" step="1" class="input input-bordered input-sm w-full">
                                    </label>
                                    <label class="form-control w-full">
                                        <div class="label"><span class="label-text" x-text="$store.global.t('label_valid_to')"></span></div>
                                        <input name="nodeForm_valid_to" x-model="nodeForm.valid_to" type="datetime-local" step="1" class="input input-bordered input-sm w-full">
                                        <div class="label"><span class="label-text-alt text-base-content/50" x-text="$store.global.t('hint_expire_auto')"></span></div>
                                    </label>
                                </div>
                            </div>
                        </template>
                    </div>
                    
                    <div class="modal-action mt-6">
                        <button class="btn" @click="nodeModal.show = false" x-text="$store.global.t('cancel')"></button>
                        <button class="btn btn-primary shadow-lg shadow-primary/20" @click="saveNode()" x-text="$store.global.t('btn_save_simple')"></button>
                    </div>
                </div>
            </dialog>
            <!-- Models Management Modal -->
            <dialog class="modal" :class="modelsModal.show ? 'modal-open' : ''">
                <div class="modal-box w-11/12 max-w-2xl">
                    <button class="btn btn-sm btn-circle btn-ghost absolute right-2 top-2"
                            @click="modelsModal.show = false">✕</button>

                    <h3 class="font-bold text-lg mb-1">
                        <span x-text="modelsModal.nodeName"></span>
                        <span class="text-base-content/40 font-normal text-sm ml-2">模型列表</span>
                    </h3>
                    <p class="text-xs text-base-content/50 mb-4">渠道内所有可用模型及其能力梯队。创建渠道时自动导入，可手动调整梯队。</p>

                    <!-- Model list table -->
                    <div class="overflow-x-auto rounded-xl border border-base-300">
                        <table class="table table-sm table-zebra w-full">
                            <thead>
                                <tr>
                                    <th>模型 ID</th>
                                    <th class="text-center">能力梯队</th>
                                    <th class="text-right">操作</th>
                                </tr>
                            </thead>
                            <tbody>
                                <template x-for="m in channelModels" :key="m.id">
                                    <tr>
                                        <td>
                                            <div class="font-mono text-sm font-semibold" x-text="m.model_id"></div>
                                            <div class="text-xs text-base-content/40" x-text="m.display_name !== m.model_id ? m.display_name : ''"></div>
                                        </td>
                                        <td class="text-center">
                                            <div class="flex items-center justify-center gap-1">
                                                <button @click="changeModelTier(m.id, 'smart')"
                                                        :class="m.capability_tier === 'smart' ? 'badge-warning' : 'badge-ghost opacity-40 hover:opacity-80'"
                                                        class="badge badge-sm cursor-pointer transition-all" title="旗舰型">🏆</button>
                                                <button @click="changeModelTier(m.id, 'fast')"
                                                        :class="m.capability_tier === 'fast' ? 'badge-info' : 'badge-ghost opacity-40 hover:opacity-80'"
                                                        class="badge badge-sm cursor-pointer transition-all" title="极速型">⚡</button>
                                                <button @click="changeModelTier(m.id, 'reasoning')"
                                                        :class="m.capability_tier === 'reasoning' ? 'badge-secondary' : 'badge-ghost opacity-40 hover:opacity-80'"
                                                        class="badge badge-sm cursor-pointer transition-all" title="沉思型">🧠</button>
                                            </div>
                                        </td>
                                        <td class="text-right">
                                            <button @click="removeModel(m.id)"
                                                    class="btn btn-ghost btn-xs text-error">移除</button>
                                        </td>
                                    </tr>
                                </template>
                                <template x-if="channelModels.length === 0">
                                    <tr>
                                        <td colspan="3" class="text-center py-8 text-base-content/40">
                                            <div class="text-2xl mb-1">📭</div>
                                            <div class="text-sm">此渠道暂无模型</div>
                                            <div class="text-xs mt-1">如果刚创建渠道，请尝试重启服务以触发自动导入</div>
                                        </td>
                                    </tr>
                                </template>
                            </tbody>
                        </table>
                    </div>

                    <!-- Add model button (for local providers like Ollama) -->
                    <div class="mt-4 flex items-center justify-between">
                        <div class="text-xs text-base-content/40">Ollama/本地模型可手动添加</div>
                        <button @click="addModelModal.show = true"
                                class="btn btn-sm btn-outline btn-success gap-1">
                            <span>+</span> 手动添加模型
                        </button>
                    </div>

                    <div class="modal-action mt-4">
                        <button class="btn" @click="modelsModal.show = false">关闭</button>
                    </div>
                </div>
                <div class="modal-backdrop" @click="modelsModal.show = false"></div>
            </dialog>

            <!-- Add Model Sub-Modal -->
            <dialog class="modal" :class="addModelModal.show ? 'modal-open' : ''">
                <div class="modal-box w-11/12 max-w-sm">
                    <button class="btn btn-sm btn-circle btn-ghost absolute right-2 top-2"
                            @click="addModelModal.show = false">✕</button>
                    <h3 class="font-bold text-base mb-4">手动添加模型</h3>
                    <div class="space-y-4">
                        <div class="form-control">
                            <div class="label pb-1"><span class="label-text font-medium">模型 ID *</span></div>
                            <input name="addModelForm_model_id" x-model="addModelForm.model_id"
                                   type="text" class="input input-bordered input-sm w-full font-mono"
                                   placeholder="e.g. qwen3:32b, llama4:70b" />
                        </div>
                        <div class="form-control">
                            <div class="label pb-2"><span class="label-text font-medium">能力梯队</span></div>
                            <div class="flex gap-2">
                                <button type="button" @click="addModelForm.capability_tier='smart'"
                                        :class="addModelForm.capability_tier==='smart'?'btn-warning':'btn-ghost'"
                                        class="btn btn-sm flex-1">🏆 旗舰</button>
                                <button type="button" @click="addModelForm.capability_tier='fast'"
                                        :class="addModelForm.capability_tier==='fast'?'btn-info':'btn-ghost'"
                                        class="btn btn-sm flex-1">⚡ 极速</button>
                                <button type="button" @click="addModelForm.capability_tier='reasoning'"
                                        :class="addModelForm.capability_tier==='reasoning'?'btn-secondary':'btn-ghost'"
                                        class="btn btn-sm flex-1">🧠 沉思</button>
                            </div>
                        </div>
                    </div>
                    <div class="modal-action mt-5">
                        <button class="btn btn-sm" @click="addModelModal.show = false">取消</button>
                        <button class="btn btn-sm btn-success" @click="addModelToChannel()">确认添加</button>
                    </div>
                </div>
                <div class="modal-backdrop" @click="addModelModal.show = false"></div>
            </dialog>
        </div>
    `
};
