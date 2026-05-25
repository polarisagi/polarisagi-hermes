import { state, t, setLang, setTheme, checkForUpdates, triggerUpdate, toggleProMode } from './store.js';
import Dashboard from './components/Dashboard.js';
import Channels from './components/Channels.js';
import Rules from './components/Rules.js';
import Clients from './components/Clients.js';
import Settings from './components/Settings.js';
import Logs from './components/Logs.js';

const { createApp, onMounted } = Vue;

const app = createApp({
    components: {
        Dashboard,
        Channels,
        Rules,
        Clients,
        Settings,
        Logs
    },
    setup() {
        onMounted(() => {
            fetch('/api/admin/info')
                .then(r => r.json())
                .then(d => {
                    state.version = d.version;
                    state.debugEnabled = d.debug;
                    checkForUpdates(d.version);
                })
                .catch(e => console.error(e));
        });

        return {
            state,
            t,
            setLang,
            setTheme,
            triggerUpdate,
            toggleProMode
        };
    }
});

app.mount('#app');