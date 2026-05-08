const { createApp, ref, computed, onMounted } = Vue;

createApp({
    setup() {
        const currentTab = ref('dashboard');
        const apiData = ref([]);
        const availableAccounts = ref([]);
        const selectedAccount = ref('all');
        const activePreset = ref('today');
        const concurrency = ref({ active: 0, waiting: 0, max: 0 });
        const startDate = ref('');
        const endDate = ref('');
        let fpInstance = null;

        const logsText = ref('Loading logs...');
        const isAutoScroll = ref(true);

        const nodes = ref([]);
        const routes = ref([]);
        const allModels = ref([]); // 所有模型（用于 datalist 建议）
        const sourceModels = ref([]); // 当前源协议对应的模型列表
        const targetModels = ref([]); // 当前目标协议对应的模型列表
        const routeModal = ref({ show: false, isEdit: false });
        const routeForm = ref({
            id: 0,
            source_protocol: 'openai',
            target_protocol: 'openai',
            model_mappings: [{ match: '', target: '' }],
            status: 1
        });

        const settings = ref({
            listen_addr: '127.0.0.1:28888',
            breaker: {
                initial_cooldown_seconds: 60,
                max_cooldown_seconds: 3600,
                failure_threshold: 3,
                failure_window_seconds: 120
            }
        });
        
        const logLevelFilter = ref('all');
        const debugEnabled = ref(false);
        const toast = ref({ show: false, message: '', type: 'success' });
        const showToast = (msg, type = 'success') => {
            toast.value = { show: true, message: msg, type };
            setTimeout(() => { toast.value.show = false }, 3000);
        };

        const nodeModal = ref({ show: false, isEdit: false });
        const nodeForm = ref({
            id: 0, provider: 'openai', name: '', credentials: '', project_id: '', location: 'global', base_url: '',
            priority: 0, limit_percent: 90.0, balance: 0.0, valid_from: '2000-01-01', valid_to: '2099-12-31', status: 1
        });

        const formatNum = (num) => Number(num).toFixed(4);
        const formatToken = (num) => new Intl.NumberFormat().format(num);
        const formatShortDate = (dt) => dt ? dt.split(' ')[0] : '-';
        const successRateColor = (rate) => rate > 95 ? 'border-emerald-500' : (rate > 80 ? 'border-yellow-500' : 'border-red-500');

        // Protocol display helpers
        const protocolLabel = (p) => {
            const labels = { openai: 'OpenAI', vertex: 'Vertex (GCP)', anthropic: 'Anthropic', gemini: 'Gemini' };
            return labels[p] || p;
        };
        const protocolClass = (p) => {
            const classes = { openai: 'text-indigo-400', vertex: 'text-emerald-400', anthropic: 'text-orange-400', gemini: 'text-blue-400' };
            return classes[p] || 'text-slate-400';
        };
        const protocolBadge = (p) => {
            const badges = { openai: 'bg-indigo-600 border-indigo-500/50', vertex: 'bg-emerald-600 border-emerald-500/50', anthropic: 'bg-orange-600 border-orange-500/50', gemini: 'bg-blue-600 border-blue-500/50' };
            return badges[p] || 'bg-slate-600 border-slate-500/50';
        };

        const selectedAccountLabel = computed(() => {
            if (selectedAccount.value === 'all') return '全部汇总';
            const matched = availableAccounts.value.find(a => a.value === selectedAccount.value);
            return matched ? matched.label : selectedAccount.value;
        });

        const groupedApiData = computed(() => {
            const map = {};
            apiData.value.forEach(r => {
                const key = r.account;
                if (!map[key]) {
                    map[key] = {
                        account: r.account, platforms: new Set(), balance: r.balance, limit_percent: r.limit_percent,
                        valid_from: r.valid_from, total_cost_usd: r.total_cost_usd, cycle_cost_usd: r.cycle_cost_usd,
                        period_cost_usd: 0, prompt_tokens: 0, completion_tokens: 0, success_count: 0, error_count: 0, breakdown: []
                    };
                }
                map[key].platforms.add(r.platform);
                map[key].period_cost_usd += r.period_cost_usd;
                map[key].prompt_tokens += r.prompt_tokens;
                map[key].completion_tokens += r.completion_tokens;
                map[key].success_count += r.success_count;
                map[key].error_count += r.error_count;
            });
            const result = Object.values(map);
            result.forEach(acc => acc.platforms = Array.from(acc.platforms));
            return result.sort((a,b) => b.period_cost_usd - a.period_cost_usd);
        });

        const singleAccountDetails = computed(() => {
            if (selectedAccount.value === 'all') return [];
            const details = apiData.value.filter(d => d.account === selectedAccount.value);
            return details.sort((a, b) => b.period_cost_usd - a.period_cost_usd);
        });

        const getUsagePercent = (row) => {
            if (!row.balance || row.balance <= 0) return 0;
            return (row.cycle_cost_usd / row.balance) * 100;
        };

        const getRemainingPercent = (row) => {
            if (!row.balance) return 100;
            const remain = row.limit_percent - getUsagePercent(row);
            return Math.max(0, remain).toFixed(2);
        };

        const getBarColor = (row) => {
            const usage = getUsagePercent(row);
            if (usage >= row.limit_percent) return 'bg-red-500';
            if (usage >= row.limit_percent * 0.85) return 'bg-yellow-500';
            return 'bg-emerald-500';
        };

        const getRemainingColor = (row) => {
            const remain = parseFloat(getRemainingPercent(row));
            if (remain <= 0) return 'text-red-400 animate-pulse';
            if (remain <= row.limit_percent * 0.15) return 'text-yellow-400';
            return 'text-emerald-400';
        };

        const fetchData = async () => {
            if (currentTab.value !== 'dashboard') return;
            try {
                const res = await fetch(`/api/stats?start=${startDate.value}&end=${endDate.value}`);
                const json = await res.json();
                apiData.value = json.details || [];
                const accSet = new Set(apiData.value.map(d => d.account));
                availableAccounts.value = Array.from(accSet).map(a => ({ account: a, label: a, value: a }));
                concurrency.value = { active: json.active_count || 0, waiting: json.waiting_count || 0, max: json.max_limit || 0 };
            } catch (e) {
                console.error("Dashboard数据抓取失败", e);
            }
        };

        const fetchSettings = async () => {
            try {
                const res = await fetch('/api/admin/settings');
                settings.value = await res.json();
            } catch (e) { console.error(e) }
        };

        const fetchNodes = async () => {
            try {
                const res = await fetch('/api/admin/nodes');
                nodes.value = await res.json() || [];
            } catch (e) { console.error(e) }
        };

        const fetchRoutes = async () => {
            try {
                const res = await fetch('/api/admin/routes');
                const data = await res.json() || [];
                // Ensure model_mappings is always an array
                data.forEach(r => {
                    if (!Array.isArray(r.model_mappings)) r.model_mappings = [];
                });
                routes.value = data;
            } catch (e) { console.error(e) }
        };

        // 从后端加载所有协议的模型列表，用于路由配置页面的模型选择建议
        const fetchAllModels = async () => {
            try {
                const res = await fetch('/api/admin/models');
                const json = await res.json();
                allModels.value = json.models || [];
            } catch (e) { console.error(e) }
        };

        // 根据协议过滤模型列表，用于路由表单中的 datalist 建议
        const getModelsForProtocol = (protocol) => {
            if (!protocol) return [];
            return allModels.value.filter(m => m.protocol === protocol);
        };

        // 当源协议改变时，更新源模型建议列表
        const onSourceProtocolChange = () => {
            sourceModels.value = getModelsForProtocol(routeForm.value.source_protocol);
        };

        // 当目标协议改变时，更新目标模型建议列表
        const onTargetProtocolChange = () => {
            targetModels.value = getModelsForProtocol(routeForm.value.target_protocol);
        };

        const saveSettings = async () => {
            if (settings.value.breaker.failure_threshold < 0 || 
                settings.value.breaker.failure_window_seconds < 0 || 
                settings.value.breaker.initial_cooldown_seconds < 0 || 
                settings.value.breaker.max_cooldown_seconds < 0) {
                showToast('各项设置的值不能为负数', 'error');
                return;
            }
            
            try {
                const payload = {
                    listen_addr: settings.value.listen_addr,
                    initial_cooldown_seconds: settings.value.breaker.initial_cooldown_seconds,
                    max_cooldown_seconds: settings.value.breaker.max_cooldown_seconds,
                    failure_threshold: settings.value.breaker.failure_threshold,
                    failure_window_seconds: settings.value.breaker.failure_window_seconds
                };
                const res = await fetch('/api/admin/settings', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                if (res.ok) {
                    showToast('系统设置已保存并热加载生效');
                } else {
                    showToast('保存失败', 'error');
                }
            } catch(e) {
                showToast('网络错误', 'error');
            }
        };

        const resetSettings = () => {
            if(!confirm('确定要恢复系统默认设置吗？')) return;
            settings.value = {
                listen_addr: '127.0.0.1:28888',
                breaker: {
                    initial_cooldown_seconds: 60,
                    max_cooldown_seconds: 3600,
                    failure_threshold: 3,
                    failure_window_seconds: 120
                }
            };
        };

        const openNodeModal = (node = null) => {
            if (node) {
                nodeForm.value = { ...node, credentials: '' };
                nodeModal.value = { show: true, isEdit: true };
            } else {
                nodeForm.value = {
                    id: 0, provider: 'openai', name: '', credentials: '', project_id: '', location: 'global', base_url: '',
                    priority: 0, limit_percent: 90.0, balance: 0.0, valid_from: '2000-01-01', valid_to: '2099-12-31', status: 1
                };
                nodeModal.value = { show: true, isEdit: false };
            }
        };

        const saveNode = async () => {
            if (!nodeForm.value.name || (!nodeModal.value.isEdit && !nodeForm.value.credentials)) {
                showToast('节点名称和API Key不能为空', 'error');
                return;
            }
            if (nodeForm.value.provider === 'vertex' && !nodeForm.value.project_id) {
                showToast('GCP Project ID 不能为空', 'error');
                return;
            }
            if (nodeForm.value.priority < 0 || nodeForm.value.balance < 0 || nodeForm.value.limit_percent < 0) {
                showToast('优先级、额度等数字不能为负数', 'error');
                return;
            }
            if (nodeForm.value.limit_percent > 100) {
                showToast('阻断水位线不能超过100', 'error');
                return;
            }
            
            const dateRegex = /^\d{4}-\d{2}-\d{2}( \d{2}:\d{2}:\d{2})?$/;
            if (nodeForm.value.valid_from && !dateRegex.test(nodeForm.value.valid_from)) {
                showToast('起止日期格式有误', 'error');
                return;
            }

            try {
                const method = nodeModal.value.isEdit ? 'PUT' : 'POST';
                const res = await fetch('/api/admin/nodes', {
                    method,
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(nodeForm.value)
                });
                if (res.ok) {
                    showToast(nodeModal.value.isEdit ? '节点已更新' : '节点已添加');
                    nodeModal.value.show = false;
                    fetchNodes();
                } else {
                    const err = await res.text();
                    showToast('保存失败: ' + err, 'error');
                }
            } catch(e) {
                showToast('网络错误', 'error');
            }
        };

        const deleteNode = async (id) => {
            if(!confirm('确定要删除这个节点吗？此操作不可恢复。')) return;
            try {
                const res = await fetch(`/api/admin/nodes?id=${id}`, { method: 'DELETE' });
                if (res.ok) {
                    showToast('节点已删除');
                    fetchNodes();
                } else {
                    showToast('删除失败', 'error');
                }
            } catch(e) {
                showToast('网络错误', 'error');
            }
        };

        // --- Route management (new protocol-to-protocol design) ---

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
            // 刷新模型列表并更新当前协议的模型建议
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
            // Validate: at least one valid mapping
            const validMappings = routeForm.value.model_mappings.filter(m => m.match.trim() !== '');
            if (validMappings.length === 0) {
                showToast('至少需要填写一个模型匹配规则', 'error');
                return;
            }
            if (!routeForm.value.source_protocol || !routeForm.value.target_protocol) {
                showToast('必须选择源协议和目标协议', 'error');
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
                    showToast(routeModal.value.isEdit ? '路由已更新' : '路由已添加');
                    routeModal.value.show = false;
                    fetchRoutes();
                } else {
                    const err = await res.text();
                    showToast('保存失败: ' + err, 'error');
                }
            } catch(e) {
                showToast('网络错误', 'error');
            }
        };

        const deleteRoute = async (id) => {
            if(!confirm('确定要删除这个路由吗？')) return;
            try {
                const res = await fetch(`/api/admin/routes?id=${id}`, { method: 'DELETE' });
                if (res.ok) {
                    showToast('路由已删除');
                    fetchRoutes();
                } else {
                    showToast('删除失败', 'error');
                }
            } catch(e) {
                showToast('网络错误', 'error');
            }
        };

        const fetchDebug = async () => {
            try {
                const res = await fetch('/api/admin/debug');
                const json = await res.json();
                debugEnabled.value = json.debug;
            } catch (e) { console.error(e) }
        };

        const toggleDebug = async () => {
            try {
                const newVal = !debugEnabled.value;
                const res = await fetch('/api/admin/debug', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ enabled: newVal })
                });
                const json = await res.json();
                debugEnabled.value = json.debug;
                showToast(debugEnabled.value ? 'Debug 模式已开启' : 'Debug 模式已关闭');
            } catch(e) {
                showToast('切换 Debug 失败', 'error');
            }
        };

        const fetchLogs = async () => {
            if (currentTab.value !== 'logs') return;
            try {
                const res = await fetch('/api/admin/logs');
                const rawText = await res.text();
                let lines = rawText.split('\n');
                if (logLevelFilter.value !== 'all') {
                    const levelStr = `level=${logLevelFilter.value.toUpperCase()}`;
                    lines = lines.filter(line => line.includes(levelStr) || line.trim() === '');
                }
                logsText.value = lines.join('\n');
                
                if (isAutoScroll.value) {
                    Vue.nextTick(() => {
                        const container = document.getElementById('logContainer');
                        if (container) container.scrollTop = container.scrollHeight;
                    });
                }
            } catch (e) {
                console.error("Fetch logs failed", e);
            }
        };

        const updateDateRange = (start, end, presetName) => {
            startDate.value = start; endDate.value = end; activePreset.value = presetName;
            if (fpInstance) fpInstance.setDate([start, end]);
            fetchData();
        };

        const formatDate = (date) => {
            const d = new Date(date);
            return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
        };

        const setPreset = (preset) => {
            const today = new Date(); let start = new Date();
            if (preset === 'today') start = today;
            else if (preset === 'week') start.setDate(today.getDate() - 6);
            else if (preset === 'month') start = new Date(today.getFullYear(), today.getMonth(), 1);
            updateDateRange(formatDate(start), formatDate(today), preset);
        };

        const aggregatedData = computed(() => {
            let source = apiData.value;
            if (selectedAccount.value !== 'all') {
                source = source.filter(d => d.account === selectedAccount.value);
            }
            let tCost = 0, tPrompt = 0, tComp = 0, tErr = 0, tSucc = 0;
            source.forEach(row => {
                tCost += row.period_cost_usd; tPrompt += row.prompt_tokens;
                tComp += row.completion_tokens; tErr += row.error_count; tSucc += row.success_count;
            });
            let rate = 0;
            if (tSucc + tErr > 0) rate = ((tSucc / (tSucc + tErr)) * 100).toFixed(2);
            return { totalCost: tCost, totalPrompt: tPrompt, totalCompletion: tComp, totalError: tErr, totalSuccess: tSucc, successRate: rate };
        });

        Vue.watch(currentTab, (newTab) => {
            if (newTab === 'settings') fetchSettings();
            if (newTab === 'nodes') fetchNodes();
            if (newTab === 'routes') {
                fetchNodes();
                fetchRoutes();
                fetchAllModels();
            }
            if (newTab === 'dashboard') fetchData();
            if (newTab === 'logs') {
                fetchLogs();
                fetchDebug();
            }
        });

        onMounted(() => {
            fpInstance = flatpickr("#datePicker", {
                mode: "range", dateFormat: "Y-m-d", locale: "zh",
                onChange: (selectedDates) => {
                    if (selectedDates.length === 2) {
                        activePreset.value = 'custom';
                        startDate.value = formatDate(selectedDates[0]);
                        endDate.value = formatDate(selectedDates[1]);
                        fetchData();
                    }
                }
            });
            setPreset('today');
            setInterval(() => {
                fetchData();
                fetchLogs();
            }, 3000);
        });

        return {
            currentTab, apiData, availableAccounts, selectedAccount, selectedAccountLabel, activePreset, groupedApiData, singleAccountDetails,
            setPreset, aggregatedData, formatNum, formatToken, formatShortDate, successRateColor, concurrency,
            getUsagePercent, getRemainingPercent, getBarColor, getRemainingColor,
            settings, nodes, routes, fetchSettings, fetchNodes, fetchRoutes, saveSettings, resetSettings,
            nodeModal, nodeForm, openNodeModal, saveNode, deleteNode,
            routeModal, routeForm, openRouteModal, saveRoute, deleteRoute, toast,
            addMapping, removeMapping, protocolLabel, protocolClass, protocolBadge,
            logsText, isAutoScroll, logLevelFilter, debugEnabled, toggleDebug, fetchLogs,
            allModels, sourceModels, targetModels, fetchAllModels, onSourceProtocolChange, onTargetProtocolChange
        };
    }
}).mount('#app');
