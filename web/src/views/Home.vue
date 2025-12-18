<template>
  <div class="home">
    <h1 class="page-title">Pool Dashboard</h1>

    <!-- Stats Cards -->
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-icon hashrate-icon">
          <!-- Pool Hashrate Icon: Speedometer/Gauge -->
          <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8z" fill="currentColor" opacity="0.3"/>
            <path d="M12 6v2M6 12H4M20 12h-2M7.76 7.76l1.42 1.42M16.24 7.76l-1.42 1.42" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
            <path d="M12 12l3.5-3.5" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"/>
            <circle cx="12" cy="12" r="2" fill="currentColor"/>
          </svg>
        </div>
        <div class="stat-content">
          <span class="stat-label">Pool Hashrate</span>
          <span class="stat-value">{{ formatHashrate(poolStore.poolHashrate) }}</span>
        </div>
      </div>

      <div class="stat-card">
        <div class="stat-icon miners-icon">
          <!-- Miners/Workers Icon: Team/People -->
          <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <circle cx="9" cy="7" r="3" fill="currentColor" opacity="0.3"/>
            <circle cx="9" cy="7" r="2" stroke="currentColor" stroke-width="2"/>
            <path d="M3 19v-1c0-2.21 2.69-4 6-4s6 1.79 6 4v1" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
            <circle cx="17" cy="8" r="2" stroke="currentColor" stroke-width="1.5"/>
            <path d="M17 12c2.21 0 4 1.34 4 3v1" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
          </svg>
        </div>
        <div class="stat-content">
          <span class="stat-label">Miners / Workers</span>
          <span class="stat-value">{{ formatNumber(poolStore.minerCount) }} / {{ formatNumber(poolStore.workerCount) }}</span>
        </div>
      </div>

      <div class="stat-card">
        <div class="stat-icon blocks-icon">
          <!-- Blocks Found Icon: 3D Cube -->
          <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M12 2L3 7v10l9 5 9-5V7l-9-5z" fill="currentColor" opacity="0.15"/>
            <path d="M12 2L3 7l9 5 9-5-9-5z" stroke="currentColor" stroke-width="2" stroke-linejoin="round"/>
            <path d="M3 7v10l9 5V12" stroke="currentColor" stroke-width="2" stroke-linejoin="round"/>
            <path d="M21 7v10l-9 5V12" stroke="currentColor" stroke-width="2" stroke-linejoin="round"/>
            <path d="M12 12v10" stroke="currentColor" stroke-width="2"/>
          </svg>
        </div>
        <div class="stat-content">
          <span class="stat-label">Blocks Found</span>
          <span class="stat-value">{{ formatNumber(poolStore.blocksFound) }}</span>
        </div>
      </div>

      <div class="stat-card">
        <div class="stat-icon network-icon">
          <!-- Network Hashrate Icon: Globe with connections -->
          <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <circle cx="12" cy="12" r="9" stroke="currentColor" stroke-width="2" opacity="0.3"/>
            <ellipse cx="12" cy="12" rx="4" ry="9" stroke="currentColor" stroke-width="1.5"/>
            <path d="M3 12h18" stroke="currentColor" stroke-width="1.5"/>
            <path d="M4.5 7h15M4.5 17h15" stroke="currentColor" stroke-width="1.5" opacity="0.6"/>
            <circle cx="12" cy="12" r="2" fill="currentColor"/>
          </svg>
        </div>
        <div class="stat-content">
          <span class="stat-label">Network Hashrate</span>
          <span class="stat-value">{{ formatHashrate(poolStore.networkHashrate) }}</span>
        </div>
      </div>
    </div>

    <!-- Charts Section -->
    <div class="charts-grid">
      <HashrateChart
        title="Pool Hashrate"
        :data="hashrateHistory"
        color="#6366f1"
        :fetchData="fetchPoolHashrate"
      />
      <WorkersChart
        title="Active Workers"
        :data="workersHistory"
      />
    </div>

    <!-- Network Info -->
    <div class="section">
      <h2 class="section-title">Network Status</h2>
      <div class="info-grid">
        <div class="info-item">
          <span class="info-label">Block Height</span>
          <span class="info-value">{{ formatNumber(poolStore.networkHeight) }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">Network Difficulty</span>
          <span class="info-value">{{ formatNumber(poolStore.networkDifficulty) }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">Last Block Found</span>
          <span class="info-value">{{ lastBlockTime }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">Total Paid</span>
          <span class="info-value">{{ formatTOS(poolStore.stats?.pool?.total_paid) }}</span>
        </div>
      </div>
    </div>

    <!-- Miner Lookup -->
    <div class="section">
      <h2 class="section-title">Lookup Your Stats</h2>
      <div class="miner-lookup">
        <input
          v-model="minerAddress"
          type="text"
          placeholder="Enter your TOS wallet address (tos1...)"
          class="lookup-input"
          @keyup.enter="lookupMiner"
        />
        <button class="lookup-btn" @click="lookupMiner" :disabled="!minerAddress">
          Lookup
        </button>
      </div>
    </div>

    <!-- Recent Blocks -->
    <div class="section">
      <div class="section-header">
        <h2 class="section-title">Recent Blocks</h2>
        <router-link to="/blocks" class="view-all">View All</router-link>
      </div>
      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>Height</th>
              <th>Hash</th>
              <th>Finder</th>
              <th>Reward</th>
              <th>Time</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="block in recentBlocks" :key="block.height">
              <td>{{ formatNumber(block.height) }}</td>
              <td class="hash-cell">{{ shortenAddress(block.hash, 10) }}</td>
              <td class="address-cell">
                <router-link :to="`/miners/${block.finder}`">
                  {{ shortenAddress(block.finder) }}
                </router-link>
              </td>
              <td>{{ formatTOS(block.reward) }}</td>
              <td>{{ formatTimeAgo(block.timestamp) }}</td>
              <td>
                <span :class="['status-badge', block.status]">{{ block.status }}</span>
              </td>
            </tr>
            <tr v-if="recentBlocks.length === 0">
              <td colspan="6" class="empty-row">No blocks found yet</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Connection Info -->
    <div class="section">
      <h2 class="section-title">Connection Details</h2>
      <div class="connection-info">
        <div class="connection-item">
          <span class="connection-label">Stratum URL</span>
          <code class="connection-value">stratum+tcp://pool.tos.network:3333</code>
        </div>
        <div class="connection-item">
          <span class="connection-label">Stratum TLS</span>
          <code class="connection-value">stratum+ssl://pool.tos.network:3334</code>
        </div>
        <div class="connection-item">
          <span class="connection-label">Username</span>
          <code class="connection-value">YOUR_TOS_ADDRESS.WORKER_NAME</code>
        </div>
        <div class="connection-item">
          <span class="connection-label">Password</span>
          <code class="connection-value">x</code>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { usePoolStore } from '../store/pool'
import { poolApi, formatHashrate, formatNumber, formatTOS, formatTimeAgo, shortenAddress } from '../services/api'
import HashrateChart from '../components/HashrateChart.vue'
import WorkersChart from '../components/WorkersChart.vue'

const router = useRouter()
const poolStore = usePoolStore()
const minerAddress = ref('')
const hashrateHistory = ref([])
const workersHistory = ref([])
let refreshInterval = null
let chartInterval = null

const recentBlocks = computed(() => {
  return poolStore.blocks.slice(0, 5)
})

const lastBlockTime = computed(() => {
  const timestamp = poolStore.stats?.pool?.last_block_found
  if (!timestamp) return 'N/A'
  return formatTimeAgo(timestamp)
})

const lookupMiner = () => {
  if (minerAddress.value) {
    router.push(`/miners/${minerAddress.value}`)
  }
}

const fetchPoolHashrate = async (period) => {
  return await poolApi.getPoolHashrateHistory(period)
}

const fetchWorkersHistory = async () => {
  workersHistory.value = await poolApi.getWorkersHistory('24h')
}

onMounted(async () => {
  refreshInterval = poolStore.startAutoRefresh(10000)
  await poolStore.fetchBlocks()

  // Fetch initial chart data
  hashrateHistory.value = await poolApi.getPoolHashrateHistory('24h')
  await fetchWorkersHistory()

  // Refresh workers chart every 30 seconds
  chartInterval = setInterval(fetchWorkersHistory, 30000)
})

onUnmounted(() => {
  if (refreshInterval) {
    clearInterval(refreshInterval)
  }
  if (chartInterval) {
    clearInterval(chartInterval)
  }
})
</script>
