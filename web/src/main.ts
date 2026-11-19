import{createApp}from'vue';import{createPinia}from'pinia';import{createRouter,createWebHashHistory}from'vue-router';import App from'./App.vue';import Overview from'./pages/Overview.vue';import Inputs from'./pages/Inputs.vue';import Speakers from'./pages/Speakers.vue';import River from'./pages/River.vue';import Settings from'./pages/Settings.vue';import Plus from'./pages/Plus.vue';import'./styles/main.css';
import './styles/workspace.css';

document.documentElement.dataset.reduceMotion = String(localStorage.getItem('air2mi-reduce-motion') === '1');
const router=createRouter({history:createWebHashHistory(),scrollBehavior: (_to, _from, saved) => saved ?? { top: 0 }, routes:[{path:'/',component:Overview},{path:'/inputs',component:Inputs},{path:'/speakers',component:Speakers},{path:'/river',component:River},{path:'/settings',component:Settings},{path:'/plus',component:Plus}]});createApp(App).use(createPinia()).use(router).mount('#app');
