import { createApp } from 'vue';
import App from './App.vue';
import router from './router';

// Naive UI
import naive from 'naive-ui';

const app = createApp(App);
app.use(router);
app.use(naive);

app.mount('#app');