export default {
    name: 'settingsComponent',
    setup() {
        return {
            async fetchSettings() {
                if (Alpine.store('global').currentTab !== 'settings') return;
                try {
                    const res = await fetch('/api/admin/settings');
                    Alpine.store('global').settings = await res.json();
                } catch (e) { console.error(e) }
            },

            async saveSettings() {
                const gStore = Alpine.store('global');
                if (gStore.settings.breaker.failure_threshold < 0 || 
                    gStore.settings.breaker.failure_window_seconds < 0 || 
                    gStore.settings.breaker.initial_cooldown_seconds < 0 || 
                    gStore.settings.breaker.max_cooldown_seconds < 0) {
                    gStore.showToast(gStore.t('err_negative'), 'error');
                    return;
                }
                
                try {
                    const payload = {
                        listen_addr: gStore.settings.listen_addr,
                        initial_cooldown_seconds: gStore.settings.breaker.initial_cooldown_seconds,
                        max_cooldown_seconds: gStore.settings.breaker.max_cooldown_seconds,
                        failure_threshold: gStore.settings.breaker.failure_threshold,
                        failure_window_seconds: gStore.settings.breaker.failure_window_seconds,
                        google_oauth_client_id: gStore.settings.google_oauth_client_id || '',
                        google_oauth_client_secret: gStore.settings.google_oauth_client_secret || ''
                    };
                    const res = await fetch('/api/admin/settings', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify(payload)
                    });
                    if (res.ok) {
                        gStore.showToast(gStore.t('settings_saved'));
                    } else {
                        gStore.showToast(gStore.t('save_failed'), 'error');
                    }
                } catch(e) {
                    gStore.showToast(gStore.t('network_error'), 'error');
                }
            },

            resetSettings() {
                const gStore = Alpine.store('global');
                if(!confirm(gStore.lang === 'zh' ? '确定要恢复系统默认设置吗？' : 'Are you sure you want to reset to default settings?')) return;
                gStore.settings = {
                    listen_addr: '127.0.0.1:28888',
                    breaker: {
                        initial_cooldown_seconds: 60,
                        max_cooldown_seconds: 3600,
                        failure_threshold: 3,
                        failure_window_seconds: 120
                    },
                    google_oauth_client_id: '',
                    google_oauth_client_secret: ''
                };
            },

            get redirectUri() {
                return window.location.protocol + '//' + window.location.host + '/api/admin/oauth/google/callback';
            },

            init() {
                this.fetchSettings();
                
                this.$watch('$store.global.currentTab', (newTab) => {
                    if (newTab === 'settings') {
                        this.fetchSettings();
                    }
                });
            }
        };
    },
    template: \`
        <div x-show="$store.global.currentTab === 'settings'" class="max-w-3xl mx-auto w-full">
            <h2 class="text-3xl font-bold mb-6" x-text="$store.global.t('tab_settings_title')"></h2>
            
            <div class="card bg-base-100 shadow-lg p-8 space-y-8">
                <div>
                    <h3 class="text-xl font-bold mb-4 border-b border-base-300 pb-2" x-text="$store.global.t('settings_basic')"></h3>
                    <div class="space-y-4">
                        <label class="form-control w-full">
                            <div class="label"><span class="label-text font-medium" x-text="$store.global.t('listen_addr_label')"></span></div>
                            <input x-model="$store.global.settings.listen_addr" type="text" class="input input-bordered w-full">
                            <div class="label"><span class="label-text-alt text-base-content/50" x-text="$store.global.t('listen_addr_hint')"></span></div>
                        </label>
                    </div>
                </div>

                <div>
                    <h3 class="text-xl font-bold mb-4 border-b border-base-300 pb-2" x-text="$store.global.t('cb_rules')"></h3>
                    <div class="grid grid-cols-2 gap-4">
                        <label class="form-control w-full">
                            <div class="label"><span class="label-text" x-text="$store.global.t('cb_threshold_label')"></span></div>
                            <input x-model.number="$store.global.settings.breaker.failure_threshold" type="number" min="0" class="input input-bordered w-full">
                        </label>
                        <label class="form-control w-full">
                            <div class="label"><span class="label-text" x-text="$store.global.t('cb_failure_window')"></span></div>
                            <input x-model.number="$store.global.settings.breaker.failure_window_seconds" type="number" min="0" class="input input-bordered w-full">
                        </label>
                        <label class="form-control w-full">
                            <div class="label"><span class="label-text" x-text="$store.global.t('cb_initial_cooldown')"></span></div>
                            <input x-model.number="$store.global.settings.breaker.initial_cooldown_seconds" type="number" min="0" class="input input-bordered w-full">
                        </label>
                        <label class="form-control w-full">
                            <div class="label"><span class="label-text" x-text="$store.global.t('cb_max_cooldown')"></span></div>
                            <input x-model.number="$store.global.settings.breaker.max_cooldown_seconds" type="number" min="0" class="input input-bordered w-full">
                        </label>
                    </div>
                </div>

                <div class="pt-4 flex justify-end gap-3">
                    <button @click="resetSettings" class="btn" x-text="$store.global.t('btn_reset_default')"></button>
                    <button @click="saveSettings" class="btn btn-primary shadow-lg shadow-primary/20" x-text="$store.global.t('btn_save_settings')"></button>
                </div>

                <div class="border-t border-base-300 pt-8 mt-6">
                    <h3 class="text-xl font-bold mb-4" x-text="$store.global.t('section_google_oauth')"></h3>
                    <p class="text-sm text-base-content/60 mb-4 leading-loose">
                        <span x-text="$store.global.t('oauth_hint_1')"></span>
                        <a href="https://console.cloud.google.com/apis/credentials" target="_blank" class="link link-info" x-text="$store.global.t('oauth_hint_link')"></a>
                        <span x-text="$store.global.t('oauth_hint_2')"></span><br>
                        <span x-text="$store.global.t('oauth_hint_3')"></span>
                        <code class="bg-base-200 px-2 py-1 rounded text-success ml-2" x-text="redirectUri"></code>
                    </p>
                    <div class="space-y-4">
                        <label class="form-control w-full">
                            <div class="label"><span class="label-text font-medium" x-text="$store.global.t('oauth_client_id')"></span></div>
                            <input x-model="$store.global.settings.google_oauth_client_id" type="text" :placeholder="$store.global.t('placeholder_gcloud_id')" class="input input-bordered w-full font-mono text-sm">
                        </label>
                        <label class="form-control w-full">
                            <div class="label"><span class="label-text font-medium" x-text="$store.global.t('oauth_client_secret')"></span></div>
                            <input x-model="$store.global.settings.google_oauth_client_secret" type="password" :placeholder="$store.global.t('placeholder_gcloud_secret')" class="input input-bordered w-full font-mono text-sm">
                        </label>
                    </div>
                </div>
            </div>
        </div>
    \`
};
