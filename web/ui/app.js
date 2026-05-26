import './store.js?v=3';
import Dashboard from './components/Dashboard.js?v=3';
import Channels from './components/Channels.js?v=3';
import Rules from './components/Rules.js?v=3';
import Clients from './components/Clients.js?v=3';
import Settings from './components/Settings.js?v=3';
import Logs from './components/Logs.js?v=3';

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
