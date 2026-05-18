import { state, t, setLang, setTheme, checkForUpdates, triggerUpdate } from './store.js';
import Dashboard from './components/Dashboard.js';
import Nodes from './components/Nodes.js';
import Routes from './components/Routes.js';
import Settings from './components/Settings.js';
import Logs from './components/Logs.js';

const { createApp, onMounted } = Vue;

const app = createApp({
    components: {
        Dashboard,
        Nodes,
        Routes,
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
            triggerUpdate
        };
    }
});

app.mount('#app');