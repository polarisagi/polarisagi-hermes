import { state, t, formatNum, formatToken, formatShortDate, successRateColor } from '../store.js';

const { computed, onMounted, onUnmounted } = Vue;

export default {
    name: 'Dashboard',
    setup() {
        const selectedAccountLabel = computed(() => {
            if (state.selectedAccount === 'all') return t('all_summary');
            const matched = state.availableAccounts.find(a => a.value === state.selectedAccount);
            return matched ? matched.label : state.selectedAccount;
        });

        const groupedApiData = computed(() => {
            const map = {};
            state.apiData.forEach(r => {
                const key = r.account;
                if (!map[key]) {
                    map[key] = {
                        account: r.account, platforms: new Set(), balance: r.balance, limit_percent: r.limit_percent,
                        valid_from: r.valid_from, total_cost_usd: r.total_cost_usd, cycle_cost_usd: r.cycle_cost_usd,
                        period_cost_usd: 0, prompt_tokens: 0, completion_tokens: 0, success_count: 0, error_count: 0, breakdown: [],
                        platformDetails: []
                    };
                }
                map[key].platforms.add(r.platform);
                map[key].period_cost_usd += r.period_cost_usd;
                map[key].prompt_tokens += r.prompt_tokens;
                map[key].completion_tokens += r.completion_tokens;
                map[key].success_count += r.success_count;
                map[key].error_count += r.error_count;
                
                // Aggregate by platform
                let pd = map[key].platformDetails.find(p => p.platform === r.platform);
                if (!pd) {
                    pd = { platform: r.platform, cost: 0 };
                    map[key].platformDetails.push(pd);
                }
                pd.cost += r.period_cost_usd;
            });
            const result = Object.values(map);
            result.forEach(acc => acc.platforms = Array.from(acc.platforms));
            return result.sort((a,b) => b.period_cost_usd - a.period_cost_usd);
        });

        const singleAccountDetails = computed(() => {
            if (state.selectedAccount === 'all') return [];
            const details = state.apiData.filter(d => d.account === state.selectedAccount);
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

        const aggregatedData = computed(() => {
            let source = state.apiData;
            if (state.selectedAccount !== 'all') {
                source = source.filter(d => d.account === state.selectedAccount);
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

        let fpInstance = null;
        let dataInterval = null;

        const fetchData = async () => {
            if (state.currentTab !== 'dashboard') return;
            try {
                const res = await fetch(\`/api/stats?start=\${state.startDate}&end=\${state.endDate}\`);
                const json = await res.json();
                state.apiData = json.details || [];
                const accSet = new Set(state.apiData.map(d => d.account));
                state.availableAccounts = Array.from(accSet).map(a => ({ account: a, label: a, value: a }));
                state.concurrency = { active: json.active_count || 0, waiting: json.waiting_count || 0, max: json.max_limit || 0 };
            } catch (e) {
                console.error(t("dashboard_fetch_failed"), e);
            }
        };

        const updateDateRange = (start, end, presetName) => {
            state.startDate = start; 
            state.endDate = end; 
            state.activePreset = presetName;
            if (fpInstance) fpInstance.setDate([start, end]);
            fetchData();
        };

        const formatDate = (date) => {
            const d = new Date(date);
            return \`\${d.getFullYear()}-\${String(d.getMonth() + 1).padStart(2, '0')}-\${String(d.getDate()).padStart(2, '0')}\`;
        };

        const setPreset = (preset) => {
            const today = new Date(); let start = new Date();
            if (preset === 'today') start = today;
            else if (preset === 'week') start.setDate(today.getDate() - 6);
            else if (preset === 'month') start = new Date(today.getFullYear(), today.getMonth(), 1);
            updateDateRange(formatDate(start), formatDate(today), preset);
        };

        onMounted(() => {
            fpInstance = flatpickr("#datePicker", {
                mode: "range", dateFormat: "Y-m-d", locale: "zh",
                onChange: (selectedDates) => {
                    if (selectedDates.length === 2) {
                        state.activePreset = 'custom';
                        state.startDate = formatDate(selectedDates[0]);
                        state.endDate = formatDate(selectedDates[1]);
                        fetchData();
                    }
                }
            });
            setPreset('today');
            dataInterval = setInterval(() => {
                fetchData();
            }, 3000);
        });

        onUnmounted(() => {
            if (dataInterval) clearInterval(dataInterval);
        });

        return {
            state, t, formatNum, formatToken, formatShortDate, successRateColor,
            selectedAccountLabel, groupedApiData, singleAccountDetails,
            getUsagePercent, getRemainingPercent, getBarColor, getRemainingColor,
            aggregatedData, setPreset
        };
    },
    template: \`
            <div v-show="state.currentTab === 'dashboard'" class="max-w-6xl mx-auto">
                <div class="flex justify-between items-start mb-8 border-b border-gray-300 dark:border-slate-700 pb-6">
                    <div>
                        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t("tab_dashboard_title") }}</h2>
                        <p class="text-gray-500 dark:text-slate-400 mt-1">{{ t("dashboard_subtitle") }}</p>
                    </div>
                    <div class="flex gap-4">
                        <div class="card rounded-xl px-5 py-3 flex flex-col items-center shadow-lg">
                            <span class="text-[10px] text-gray-500 dark:text-slate-500 uppercase font-bold tracking-wider mb-1">{{ t("processing") }}</span>
                            <div class="flex items-baseline gap-1">
                                <span class="text-2xl font-mono text-emerald-400">{{ state.concurrency.active }}</span>
                                <span class="text-xs text-gray-500 dark:text-slate-500">/ {{ state.concurrency.max }}</span>
                            </div>
                        </div>
                        <div class="card rounded-xl px-5 py-3 flex flex-col items-center shadow-lg"
                            :class="state.concurrency.waiting > 0 ? 'border-yellow-500/50 ring-1 ring-yellow-500/20' : ''">
                            <span class="text-[10px] text-gray-500 dark:text-slate-500 uppercase font-bold tracking-wider mb-1">{{ t("waiting") }}</span>
                            <div class="flex items-center gap-2">
                                <span class="text-2xl font-mono"
                                    :class="state.concurrency.waiting > 0 ? 'text-yellow-400 pulse' : 'text-gray-400 dark:text-slate-600'">
                                    {{ state.concurrency.waiting }}
                                </span>
                            </div>
                        </div>
                        <div class="flex flex-col justify-end">
                            <select v-model="state.selectedAccount" class="bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-600 text-gray-900 dark:text-white text-sm rounded-lg block p-2.5 outline-none focus:ring-2 focus:ring-blue-500">
                                <option value="all">{{ t("view_all_protocols") }}</option>
                                <option v-for="acc in state.availableAccounts" :key="acc.value" :value="acc.value">{{ acc.label }}</option>
                            </select>
                        </div>
                    </div>
                </div>

                <div class="card rounded-xl p-4 mb-8 flex flex-wrap gap-4 items-center justify-between shadow-lg">
                    <div class="flex gap-2">
                        <button @click="setPreset('today')" :class="state.activePreset === 'today' ? 'bg-blue-600 text-white' : 'bg-gray-200 dark:bg-slate-700 text-gray-700 dark:text-slate-300'" class="px-4 py-2 rounded-lg text-sm transition">{{ t("filter_today") }}</button>
                        <button @click="setPreset('week')" :class="state.activePreset === 'week' ? 'bg-blue-600 text-white' : 'bg-gray-200 dark:bg-slate-700 text-gray-700 dark:text-slate-300'" class="px-4 py-2 rounded-lg text-sm transition">{{ t("filter_7days") }}</button>
                        <button @click="setPreset('month')" :class="state.activePreset === 'month' ? 'bg-blue-600 text-white' : 'bg-gray-200 dark:bg-slate-700 text-gray-700 dark:text-slate-300'" class="px-4 py-2 rounded-lg text-sm transition">{{ t("filter_month") }}</button>
                    </div>
                    <div class="flex items-center gap-3">
                        <span class="text-gray-500 dark:text-slate-400 text-sm">{{ t("custom_range") }}</span>
                        <input type="text" id="datePicker" :placeholder="t('select_date_range')" class="bg-white dark:bg-slate-800 border border-gray-300 dark:border-slate-600 text-gray-900 dark:text-white text-sm rounded-lg block w-64 p-2.5 text-center outline-none focus:ring-2 focus:ring-blue-500">
                    </div>
                </div>

                <div class="grid grid-cols-1 md:grid-cols-4 gap-6 mb-8">
                    <div class="card rounded-xl p-6 shadow-lg border-l-4 border-emerald-500">
                        <p class="text-gray-500 dark:text-slate-400 text-sm mb-1">{{ t("estimated_cost") }}</p>
                        <h3 class="text-3xl font-bold text-gray-900 dark:text-white">$ {{ formatNum(aggregatedData.totalCost) }}</h3>
                    </div>
                    <div class="card rounded-xl p-6 shadow-lg border-l-4 border-blue-500">
                        <p class="text-gray-500 dark:text-slate-400 text-sm mb-1">{{ t("prompt_tokens") }}</p>
                        <h3 class="text-2xl font-bold text-blue-400">{{ formatToken(aggregatedData.totalPrompt) }}</h3>
                    </div>
                    <div class="card rounded-xl p-6 shadow-lg border-l-4 border-purple-500">
                        <p class="text-gray-500 dark:text-slate-400 text-sm mb-1">{{ t("completion_tokens") }}</p>
                        <h3 class="text-2xl font-bold text-purple-400">{{ formatToken(aggregatedData.totalCompletion) }}</h3>
                    </div>
                    <div class="card rounded-xl p-6 shadow-lg border-l-4" :class="successRateColor(aggregatedData.successRate)">
                        <p class="text-gray-500 dark:text-slate-400 text-sm mb-1">{{ t("success_rate") }}</p>
                        <div class="flex items-end gap-2">
                            <h3 class="text-2xl font-bold text-gray-900 dark:text-white">{{ aggregatedData.successRate }}%</h3>
                        </div>
                    </div>
                </div>

                <div v-if="state.selectedAccount === 'all'" class="card rounded-xl shadow-lg overflow-hidden">
                    <table class="w-full text-sm text-left text-gray-700 dark:text-slate-300">
                        <thead class="text-xs text-gray-500 dark:text-slate-400 uppercase bg-white dark:bg-slate-800 border-b border-gray-300 dark:border-slate-700">
                            <tr>
                                <th class="px-6 py-4">{{ t("matrix_header_node") }}</th>
                                <th class="px-6 py-4 text-right whitespace-nowrap">{{ t("matrix_header_cost") }}</th>
                                <th class="px-6 py-4 text-center">{{ t("matrix_header_level") }}</th>
                                <th class="px-6 py-4 text-right">{{ t("matrix_header_tokens") }}</th>
                                <th class="px-6 py-4 text-center">{{ t("matrix_header_status") }}</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr v-for="row in groupedApiData" :key="row.account" class="border-b border-gray-300 dark:border-slate-700 hover:bg-gray-200 dark:bg-slate-700/50">
                                <td class="px-6 py-4">
                                    <div class="font-medium text-blue-300 text-lg mb-2">{{ row.account }}</div>
                                    <div class="space-y-3 pr-2">
                                        <div v-for="pd in row.platformDetails" :key="pd.platform" class="bg-white dark:bg-slate-800/50 rounded-lg p-2 border border-gray-300 dark:border-slate-700/50">
                                            <div class="text-xs font-bold text-gray-700 dark:text-slate-300 mb-2 uppercase flex justify-between items-center">
                                                <span :class="pd.platform === 'openai' ? 'text-indigo-400' : 'text-emerald-400'">{{ pd.platform }} {{ t("protocol_suffix") }}</span>
                                                <span class="text-emerald-400 font-mono">\${{ formatNum(pd.cost) }}</span>
                                            </div>
                                        </div>
                                    </div>
                                </td>
                                <td class="px-6 py-4 text-right font-bold text-emerald-400 align-top pt-5">$ {{ formatNum(row.period_cost_usd) }}</td>
                                <td class="px-6 py-4 align-top pt-5">
                                    <div v-if="row.balance > 0">
                                        <div class="flex justify-between text-[10px] mb-1">
                                            <span class="text-gray-500 dark:text-slate-400">{{ t("from_date") }} {{ formatShortDate(row.valid_from) }}</span>
                                            <span :class="getRemainingColor(row)" class="font-semibold">{{ t("remaining") }} {{ getRemainingPercent(row) }}%</span>
                                        </div>
                                        <div class="w-full bg-white dark:bg-slate-800 rounded-full h-2.5 relative overflow-hidden ring-1 ring-gray-300 dark:ring-slate-700">
                                            <div :class="getBarColor(row)" class="h-2.5 rounded-full transition-all duration-700 ease-in-out" :style="{ width: Math.min(getUsagePercent(row), 100) + '%' }"></div>
                                        </div>
                                        <div class="text-[10px] text-gray-500 dark:text-slate-500 mt-1 flex justify-between">
                                            <span>{{ t("this_cycle") }} \${{ formatNum(row.cycle_cost_usd) }}</span>
                                            <span>{{ t("total_balance") }} \${{ formatNum(row.balance) }}</span>
                                        </div>
                                    </div>
                                    <div v-else class="text-[10px] text-gray-500 dark:text-slate-500 bg-white dark:bg-slate-800 px-2 py-1 rounded inline-block text-center w-full">{{ t("no_limit") }}</div>
                                </td>
                                <td class="px-6 py-4 text-right align-top pt-5">{{ formatToken(row.prompt_tokens + row.completion_tokens) }}</td>
                                <td class="px-6 py-4 text-center align-top pt-5">
                                    <span class="bg-emerald-900 text-emerald-300 text-xs px-2 py-1 rounded mr-1">√ {{ row.success_count }}</span>
                                    <span class="bg-red-900 text-red-300 text-xs px-2 py-1 rounded">x {{ row.error_count }}</span>
                                </td>
                            </tr>
                            <tr v-if="groupedApiData.length === 0">
                                <td colspan="5" class="px-6 py-8 text-center text-gray-500 dark:text-slate-500">{{ t("no_data_range") }}</td>
                            </tr>
                        </tbody>
                    </table>
                </div>

                <div v-else class="card rounded-xl shadow-lg overflow-hidden">
                    <div class="px-6 py-4 border-b border-gray-300 dark:border-slate-700 bg-white dark:bg-slate-800/30 flex items-center justify-between">
                        <h3 class="text-lg font-semibold text-blue-300">
                            <span class="text-gray-500 dark:text-slate-400 text-sm font-normal mr-2">{{ t("current_filtered_node") }}</span>
                            {{ selectedAccountLabel }}
                        </h3>
                    </div>
                    <table class="w-full text-sm text-left text-gray-700 dark:text-slate-300">
                        <thead class="text-xs text-gray-500 dark:text-slate-400 uppercase bg-white dark:bg-slate-800 border-b border-gray-300 dark:border-slate-700">
                            <tr>
                                <th class="px-6 py-4">{{ t("detail_header_platform") }}</th>
                                <th class="px-6 py-4">{{ t("detail_header_client") }}</th>
                                <th class="px-6 py-4">{{ t("detail_header_method") }}</th>
                                <th class="px-6 py-4 text-right">{{ t("detail_header_cost") }}</th>
                                <th class="px-6 py-4 text-right">Prompt / Output</th>
                                <th class="px-6 py-4 text-center">{{ t("matrix_header_status") }}</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr v-for="row in singleAccountDetails" :key="row.platform + row.client + row.method" class="border-b border-gray-300 dark:border-slate-700 hover:bg-gray-200 dark:bg-slate-700/50">
                                <td class="px-6 py-4"><span :class="row.platform === 'openai' ? 'text-indigo-400' : 'text-emerald-400'" class="font-bold uppercase text-xs">{{ row.platform }}</span></td>
                                <td class="px-6 py-4 font-medium text-gray-700 dark:text-slate-300">{{ row.client }}</td>
                                <td class="px-6 py-4 text-blue-400 font-mono text-xs">{{ row.method }}</td>
                                <td class="px-6 py-4 text-right font-bold text-emerald-400">$ {{ formatNum(row.period_cost_usd) }}</td>
                                <td class="px-6 py-4 text-right text-gray-500 dark:text-slate-400 text-xs">{{ formatToken(row.prompt_tokens) }} / {{ formatToken(row.completion_tokens) }}</td>
                                <td class="px-6 py-4 text-center">
                                    <span class="bg-emerald-900 text-emerald-300 text-xs px-2 py-1 rounded mr-1">√ {{ row.success_count }}</span>
                                    <span class="bg-red-900 text-red-300 text-xs px-2 py-1 rounded">x {{ row.error_count }}</span>
                                </td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>
    \`
};