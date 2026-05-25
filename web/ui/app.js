import { state, t, formatNum, formatToken, formatShortDate, successRateColor, protocolLabel, protocolClass, protocolBadge, setLang, setTheme, checkForUpdates, triggerUpdate, toggleProMode } from './store.js';
import Dashboard from './components/Dashboard.js';
import Channels from './components/Channels.js';
import Rules from './components/Rules.js';
import Clients from './components/Clients.js';
import Settings from './components/Settings.js';
import Logs from './components/Logs.js';

const components = [
    Dashboard,
    Channels,
    Rules,
    Clients,
    Settings,
    Logs
];

document.addEventListener('alpine:init', () => {
    const container = document.getElementById('components-container');
    
    components.forEach(comp => {
        // 1. Register Alpine data function
        if (comp.name && comp.setup) {
            Alpine.data(comp.name, () => comp.setup());
        }
        
        // 2. Inject template string into DOM
        if (comp.template) {
            // We wrap it so we can easily attach x-data
            const wrapper = document.createElement('div');
            wrapper.setAttribute('x-data', comp.name ? `${comp.name}()` : '{}');
            wrapper.innerHTML = comp.template;
            container.appendChild(wrapper);
        }
    });
});
