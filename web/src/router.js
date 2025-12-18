import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  {
    path: '/',
    name: 'Home',
    component: () => import('./views/Home.vue')
  },
  {
    path: '/miners/:address?',
    name: 'Miners',
    component: () => import('./views/Miners.vue')
  },
  {
    path: '/blocks',
    name: 'Blocks',
    component: () => import('./views/Blocks.vue')
  },
  {
    path: '/payments',
    name: 'Payments',
    component: () => import('./views/Payments.vue')
  },
  {
    path: '/getting-started',
    name: 'GettingStarted',
    component: () => import('./views/GettingStarted.vue')
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

export default router
