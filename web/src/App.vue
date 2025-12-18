<template>
  <div class="app">
    <header class="header">
      <div class="container">
        <router-link to="/" class="logo">
          <span class="logo-icon">TOS</span>
          <span class="logo-text">Mining Pool</span>
        </router-link>
        <nav class="nav">
          <router-link to="/" class="nav-link">Dashboard</router-link>
          <router-link to="/miners" class="nav-link">Miners</router-link>
          <router-link to="/blocks" class="nav-link">Blocks</router-link>
          <router-link to="/payments" class="nav-link">Payments</router-link>
          <router-link to="/getting-started" class="nav-link">Getting Started</router-link>
        </nav>
        <button class="mobile-menu-btn" @click="mobileMenuOpen = !mobileMenuOpen">
          <span></span>
          <span></span>
          <span></span>
        </button>
      </div>
    </header>

    <div class="mobile-nav" :class="{ open: mobileMenuOpen }">
      <router-link to="/" class="nav-link" @click="mobileMenuOpen = false">Dashboard</router-link>
      <router-link to="/miners" class="nav-link" @click="mobileMenuOpen = false">Miners</router-link>
      <router-link to="/blocks" class="nav-link" @click="mobileMenuOpen = false">Blocks</router-link>
      <router-link to="/payments" class="nav-link" @click="mobileMenuOpen = false">Payments</router-link>
      <router-link to="/getting-started" class="nav-link" @click="mobileMenuOpen = false">Getting Started</router-link>
    </div>

    <main class="main">
      <div class="container">
        <router-view />
      </div>
    </main>

    <footer class="footer">
      <div class="container">
        <p>TOS Mining Pool v1.0.0 | Pool Fee: {{ poolFee }}%</p>
        <p class="footer-links">
          <a href="https://github.com/tos-network/tos-pool" target="_blank">GitHub</a>
          <span>|</span>
          <a href="https://github.com/tos-network/tosminer" target="_blank">tosminer</a>
        </p>
      </div>
    </footer>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { usePoolStore } from './store/pool'

const mobileMenuOpen = ref(false)
const poolStore = usePoolStore()
const poolFee = ref(1.0)

onMounted(async () => {
  await poolStore.fetchStats()
  if (poolStore.stats?.pool?.fee) {
    poolFee.value = poolStore.stats.pool.fee
  }
})
</script>
