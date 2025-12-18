<template>
  <div class="home">
    <h1 class="page-title">Pool Dashboard</h1>

    <!-- Stats Cards -->
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-icon hashrate-icon"></div>
        <div class="stat-content">
          <span class="stat-label">Pool Hashrate</span>
          <span class="stat-value">{{ formatHashrate(poolStore.poolHashrate) }}</span>
        </div>
      </div>

      <div class="stat-card">
        <div class="stat-icon miners-icon"></div>
        <div class="stat-content">
          <span class="stat-label">Miners / Workers</span>
          <span class="stat-value">{{ formatNumber(poolStore.minerCount) }} / {{ formatNumber(poolStore.workerCount) }}</span>
        </div>
      </div>

      <div class="stat-card">
        <div class="stat-icon blocks-icon"></div>
        <div class="stat-content">
          <span class="stat-label">Blocks Found</span>
          <span class="stat-value">{{ formatNumber(poolStore.blocksFound) }}</span>
        </div>
      </div>

      <div class="stat-card">
        <div class="stat-icon network-icon"></div>
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
