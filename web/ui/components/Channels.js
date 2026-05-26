export default {
    name: 'channelsComponent',
    setup() {
        return {
            nodeModal: { show: false, isEdit: false },
            sysProviders: [],
            sysAuthModes: [],
            availableAuthModes: [],
            selectedAuthMode: null,
            nodeForm: {
                id: 0, protocol: 'openai', provider: 'openai', sys_auth_mode_id: '', name: '', credentials: '', project_id: '', location: 'global', base_url: '',
                priority: 10, limit_percent: 90.0, balance: 0.0, min_request_interval_sec: 0, concurrency: 0,
                valid_from: '', valid_to: '', status: 1
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
                    Alpine.store('global').nodes = await res.json() || [];
                } catch (e) { console.error(e) }
            },

            async fetchSysProviders() {
                try {
                    const res = await fetch('/api/admin/sys_providers');
                    const data = await res.json();
                    if (data && data.providers) {
                        this.sysProviders = data.providers;
                        this.sysAuthModes = data.auth_modes;
                    }
                } catch (e) { console.error("Failed to fetch sys_providers", e); }
            },

            openNodeModal(node = null) {
                if (node) {
                    this.nodeForm = {
                        ...node,
                        credentials: '',
                        valid_from: this.toDatetimeLocal(node.valid_from),
                        valid_to: this.toDatetimeLocal(node.valid_to),
                    };
                    this.nodeModal = { show: true, isEdit: true };
                } else {
                    const today = this.todayPrefix();
                    this.nodeForm = {
                        id: 0, protocol: 'openai', provider: 'openai', name: '', credentials: '', project_id: '', location: 'global', base_url: '',
                        priority: 10, limit_percent: 90.0, balance: 0.0, min_request_interval_sec: 0, concurrency: 0,
                        valid_from: `${today}T00:00:00`, valid_to: `2099-12-31T23:59:59`, status: 1
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
                    if (this.selectedAuthMode) {
                        if (this.selectedAuthMode.auth_type === 'adc') {
                            try {
                                authCreds = JSON.parse(form.credentials);
                            } catch(e) {
                                // Just pass as string if it's not valid JSON yet (e.g. they typed something)
                                // But usually ADC is a JSON object.
                                authCreds.adc_json = form.credentials;
                            }
                        } else if (this.selectedAuthMode.auth_type !== 'none') {
                            authCreds.api_key = form.credentials;
                        }
                        if (this.selectedAuthMode.required_fields.includes('project_id')) authCreds.project_id = form.project_id;
                        if (this.selectedAuthMode.required_fields.includes('region')) authCreds.region = form.location;
                    }
                    
                    const payload = {
                        ...form,
                        sys_provider_id: form.provider,
                        sys_auth_mode_id: form.sys_auth_mode_id,
                        auth_credentials: authCreds,
                        valid_from: this.fromDatetimeLocal(form.valid_from),
                        valid_to: this.fromDatetimeLocal(form.valid_to),
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
                this.$watch('nodeForm.protocol', (newVal) => {
                    if (!this.nodeModal.isEdit) {
                        const available = this.sysProviders.filter(x => x.api_protocol === newVal);
                        if (available.length > 0 && !available.find(x => x.provider_id === this.nodeForm.provider)) {
                            this.nodeForm.provider = available[0].provider_id;
                        }
                    }
                });
                
                // Function to update computed auth mode state
                const updateAuthModes = () => {
                    this.availableAuthModes = this.sysAuthModes.filter(m => m.provider_id === this.nodeForm.provider);
                    if (this.availableAuthModes.length > 0) {
                        // If current auth mode is not in available, select the first one
                        if (!this.availableAuthModes.find(m => m.mode_id === this.nodeForm.sys_auth_mode_id)) {
                            this.nodeForm.sys_auth_mode_id = this.availableAuthModes[0].mode_id;
                        }
                    } else {
                        this.nodeForm.sys_auth_mode_id = '';
                        this.selectedAuthMode = null;
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

                this.$watch('nodeForm.sys_auth_mode_id', (newVal) => {
                    this.selectedAuthMode = this.availableAuthModes.find(m => m.mode_id === newVal) || null;
                });
                
                // Also trigger initial calculation when modal opens
                this.$watch('nodeModal.show', (newVal) => {
                    if (newVal) {
                        updateAuthModes();
                    }
                });
                
                this.$watch('$store.global.currentTab', (newTab) => {
                    if (newTab === 'channels') this.fetchNodes();
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
                                <td colspan="7" class="text-center py-8 text-base-content/50" x-text="$store.global.t('no_nodes')"></td>
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
                                    <div class="label"><span class="label-text font-medium"><span x-text="$store.global.t('protocol_type_req')"></span> <span class="text-error">*</span></span></div>
                                    <select x-model="nodeForm.protocol" class="select select-bordered select-sm w-full">
                                        <option value="openai">OpenAI 协议</option>
                                        <option value="anthropic">Anthropic 协议</option>
                                        <option value="google">Google API 协议</option>
                                        <option value="local">本地部署协议</option>
                                    </select>
                                </label>
                                <label class="form-control w-full">
                                    <div class="label"><span class="label-text font-medium">大模型厂商 <span class="text-error">*</span></span></div>
                                    <select x-model="nodeForm.provider" class="select select-bordered select-sm w-full">
                                        <template x-for="p in sysProviders.filter(x => x.api_protocol === nodeForm.protocol)" :key="p.provider_id">
                                            <option :value="p.provider_id" x-text="p.provider_name"></option>
                                        </template>
                                    </select>
                                </label>
                            </div>
                            <div class="grid grid-cols-1 gap-4">
                                <label class="form-control w-full">
                                    <div class="label"><span class="label-text font-medium"><span x-text="$store.global.t('node_name_req')"></span> <span class="text-error">*</span></span></div>
                                    <input x-model="nodeForm.name" type="text" :placeholder="$store.global.t('placeholder_node_name')" class="input input-bordered input-sm w-full">
                                </label>
                            </div>
                            
                            <!-- 动态鉴权方式选择 (仅当厂商有多种鉴权模式时显示) -->
                            <template x-if="availableAuthModes.length > 1">
                                <div class="grid grid-cols-1 gap-4 mb-4">
                                    <label class="form-control w-full">
                                        <div class="label"><span class="label-text font-medium">鉴权方式 <span class="text-error">*</span></span></div>
                                        <div class="join">
                                            <template x-for="m in availableAuthModes" :key="m.mode_id">
                                                <input class="join-item btn btn-sm w-1/2" type="radio" :aria-label="m.mode_name" x-model="nodeForm.sys_auth_mode_id" :value="m.mode_id" />
                                            </template>
                                        </div>
                                    </label>
                                </div>
                            </template>
                            
                            <label class="form-control w-full">
                                <div class="label">
                                    <span class="label-text font-medium">
                                        <template x-if="selectedAuthMode && selectedAuthMode.auth_type === 'adc'"><span>ADC JSON</span></template>
                                        <template x-if="!selectedAuthMode || selectedAuthMode.auth_type !== 'adc'"><span>API Key</span></template>
                                        
                                        <template x-if="selectedAuthMode && selectedAuthMode.auth_type !== 'none'"><span class="text-error">*</span></template>
                                        
                                        <!-- 动态提示词 -->
                                        <template x-if="selectedAuthMode && selectedAuthMode.auth_type === 'adc'"><span class="text-base-content/50 text-xs ml-1 font-normal" x-text="$store.global.t('hint_adc_paste')"></span></template>
                                        <template x-if="selectedAuthMode && selectedAuthMode.auth_type === 'header'"><span class="text-base-content/50 text-xs ml-1 font-normal" x-text="'(放在 ' + selectedAuthMode.header_name + ' 请求头中)'"></span></template>
                                        <template x-if="selectedAuthMode && selectedAuthMode.auth_type === 'bearer'"><span class="text-base-content/50 text-xs ml-1 font-normal" x-text="$store.global.t('hint_sk_bearer')"></span></template>
                                        <template x-if="selectedAuthMode && selectedAuthMode.auth_type === 'none'"><span class="text-base-content/50 text-xs ml-1 font-normal">通常无需验证，留空即可</span></template>
                                    </span>
                                    <template x-if="selectedAuthMode && selectedAuthMode.auth_type === 'adc'">
                                        <button @click="startGoogleAuth" class="btn btn-xs btn-outline btn-info">🔑 <span x-text="$store.global.t('btn_oauth_auto')"></span></button>
                                    </template>
                                </div>
                                
                                <template x-if="selectedAuthMode && selectedAuthMode.auth_type === 'adc'">
                                    <textarea x-model="nodeForm.credentials" rows="3" :placeholder="nodeModal.isEdit ? $store.global.t('placeholder_adc_edit') : $store.global.t('placeholder_adc_new')" class="textarea textarea-bordered font-mono text-xs w-full"></textarea>
                                </template>
                                <template x-if="!selectedAuthMode || selectedAuthMode.auth_type !== 'adc'">
                                    <input x-model="nodeForm.credentials" type="password" :placeholder="nodeModal.isEdit ? $store.global.t('placeholder_key_edit') : $store.global.t('placeholder_key_new')" class="input input-bordered input-sm w-full">
                                </template>
                            </label>
                            
                            <template x-if="$store.global.proMode">
                                <div class="grid grid-cols-2 gap-4">
                                    <label class="form-control w-full">
                                        <div class="label"><span class="label-text" x-text="$store.global.t('priority')"></span></div>
                                        <input x-model.number="nodeForm.priority" type="number" min="0" class="input input-bordered input-sm w-full">
                                        <div class="label"><span class="label-text-alt text-base-content/50" x-text="$store.global.t('priority_hint')"></span></div>
                                    </label>
                                    <label class="form-control w-full">
                                        <div class="label"><span class="label-text" x-text="$store.global.t('min_interval_label')"></span></div>
                                        <input x-model.number="nodeForm.min_request_interval_sec" type="number" min="0" class="input input-bordered input-sm w-full">
                                        <div class="label"><span class="label-text-alt text-base-content/50" x-text="$store.global.t('min_interval_hint')"></span></div>
                                    </label>
                                </div>
                            </template>
                            <template x-if="$store.global.proMode">
                                <div class="grid grid-cols-2 gap-4">
                                    <label class="form-control w-full">
                                        <div class="label"><span class="label-text" x-text="$store.global.t('status')"></span></div>
                                        <select x-model.number="nodeForm.status" class="select select-bordered select-sm w-full">
                                            <option value="1" x-text="$store.global.t('status_option_enable')"></option>
                                            <option value="0" x-text="$store.global.t('status_option_disable')"></option>
                                            <option value="-1" x-text="$store.global.t('status_option_exhaust')"></option>
                                        </select>
                                    </label>
                                    <label class="form-control w-full">
                                        <div class="label"><span class="label-text">Concurrency (并发限制)</span></div>
                                        <input x-model.number="nodeForm.concurrency" type="number" min="0" max="1000" class="input input-bordered input-sm w-full">
                                        <div class="label"><span class="label-text-alt text-base-content/50">0 为无限制，上限 1000</span></div>
                                    </label>
                                </div>
                            </template>
                        </div>

                        <!-- 区块 2: 供应商配置 -->
                        <div class="bg-base-200 p-4 rounded-xl space-y-4 border border-base-300">
                            <h4 class="text-xs font-bold text-base-content/50 uppercase tracking-wider" x-text="$store.global.t('section_provider')"></h4>
                            <template x-if="selectedAuthMode && (selectedAuthMode.required_fields.includes('project_id') || selectedAuthMode.required_fields.includes('region'))">
                                <div class="grid grid-cols-2 gap-4">
                                    <template x-if="selectedAuthMode.required_fields.includes('project_id')">
                                        <label class="form-control w-full">
                                            <div class="label"><span class="label-text font-medium"><span x-text="$store.global.t('gcp_project_id')"></span> <span class="text-error">*</span></span></div>
                                            <input x-model="nodeForm.project_id" type="text" placeholder="your-gcp-project-id" class="input input-bordered input-sm w-full">
                                        </label>
                                    </template>
                                    <template x-if="selectedAuthMode.required_fields.includes('region')">
                                        <label class="form-control w-full">
                                            <div class="label"><span class="label-text" x-text="$store.global.t('gcp_location')"></span></div>
                                            <input x-model="nodeForm.location" type="text" placeholder="global" class="input input-bordered input-sm w-full">
                                            <div class="label"><span class="label-text-alt text-base-content/50" x-text="$store.global.t('hint_location')"></span></div>
                                        </label>
                                    </template>
                                </div>
                            </template>
                            <template x-if="$store.global.proMode || nodeForm.provider === 'ollama' || nodeForm.provider === 'deepseek' || nodeForm.provider === 'siliconflow' || nodeForm.provider === 'grok' || nodeForm.provider === 'openrouter'">
                                <label class="form-control w-full">
                                    <div class="label"><span class="label-text" x-text="$store.global.t('base_url_optional')"></span></div>
                                    <input x-model="nodeForm.base_url" type="text" :placeholder="$store.global.t('placeholder_baseurl')" class="input input-bordered input-sm w-full">
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
                                        <input x-model.number="nodeForm.balance" type="number" min="0" step="0.01" placeholder="0.00" class="input input-bordered input-sm w-full">
                                        <div class="label"><span class="label-text-alt text-base-content/50" x-text="$store.global.t('hint_unlimited')"></span></div>
                                    </label>
                                    <label class="form-control w-full">
                                        <div class="label"><span class="label-text" x-text="$store.global.t('label_limit_percent')"></span></div>
                                        <input x-model.number="nodeForm.limit_percent" type="number" min="0" max="100" step="0.1" class="input input-bordered input-sm w-full">
                                        <div class="label"><span class="label-text-alt text-base-content/50" x-text="$store.global.t('hint_limit_percent')"></span></div>
                                    </label>
                                    <label class="form-control w-full">
                                        <div class="label"><span class="label-text" x-text="$store.global.t('label_valid_from')"></span></div>
                                        <input x-model="nodeForm.valid_from" type="datetime-local" step="1" class="input input-bordered input-sm w-full">
                                    </label>
                                    <label class="form-control w-full">
                                        <div class="label"><span class="label-text" x-text="$store.global.t('label_valid_to')"></span></div>
                                        <input x-model="nodeForm.valid_to" type="datetime-local" step="1" class="input input-bordered input-sm w-full">
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
        </div>
    `
};
