<template>
  <div class="miners">
    <h1 class="page-title">Miner Stats</h1>

    <!-- Miner Lookup -->
    <div class="section">
      <div class="miner-lookup">
        <input
          v-model="searchAddress"
          type="text"
          placeholder="Enter TOS wallet address (tos1...)"
          class="lookup-input"
          @keyup.enter="lookupMiner"
        />
        <button class="lookup-btn" @click="lookupMiner" :disabled="!searchAddress">
          Search
        </button>
      </div>
    </div>

    <!-- Loading State -->
    <div v-if="minerStore.loading" class="loading">
      <div class="spinner"></div>
      <p>Loading miner data...</p>
    </div>

    <!-- Error State -->
    <div v-else-if="minerStore.error" class="error-box">
      <p>{{ minerStore.error }}</p>
      <p v-if="minerStore.error === 'Miner not found'">
        This address has not submitted any shares yet.
      </p>
    </div>

    <!-- Miner Data -->
    <template v-else-if="minerStore.data">
      <!-- Hashrate Chart -->
      <div class="section">
        <HashrateChart
          title="Your Hashrate"
          :data="hashrateHistory"
          color="#10b981"
          :fetchData="fetchMinerHashrate"
        />
      </div>

      <!-- Stats Cards -->
      <div class="stats-grid">
        <div class="stat-card">
          <div class="stat-content">
            <span class="stat-label">Current Hashrate</span>
            <span class="stat-value">{{ formatHashrate(minerStore.hashrate) }}</span>
          </div>
        </div>

        <div class="stat-card">
          <div class="stat-content">
            <span class="stat-label">24h Average</span>
            <span class="stat-value">{{ formatHashrate(minerStore.data?.hashrate_large) }}</span>
          </div>
        </div>

        <div class="stat-card">
          <div class="stat-content">
            <span class="stat-label">Pending Balance</span>
            <span class="stat-value">{{ formatTOS(minerStore.balance) }}</span>
          </div>
        </div>

        <div class="stat-card">
          <div class="stat-content">
            <span class="stat-label">Total Paid</span>
            <span class="stat-value">{{ formatTOS(minerStore.totalPaid) }}</span>
          </div>
        </div>
      </div>

      <!-- Miner Info -->
      <div class="section">
        <h2 class="section-title">Account Details</h2>
        <div class="info-grid">
          <div class="info-item full-width">
            <span class="info-label">Address</span>
            <span class="info-value mono">{{ minerStore.address }}</span>
          </div>
          <div class="info-item">
            <span class="info-label">Immature Balance</span>
            <span class="info-value">{{ formatTOS(minerStore.immatureBalance) }}</span>
          </div>
          <div class="info-item">
            <span class="info-label">Blocks Found</span>
            <span class="info-value">{{ formatNumber(minerStore.data?.blocks_found || 0) }}</span>
          </div>
          <div class="info-item">
            <span class="info-label">Last Share</span>
            <span class="info-value">{{ formatTimeAgo(minerStore.data?.last_share) }}</span>
          </div>
          <div class="info-item">
            <span class="info-label">Workers Online</span>
            <span class="info-value">{{ minerStore.workers.length }}</span>
          </div>
        </div>
      </div>

      <!-- Workers -->
      <div class="section" v-if="minerStore.workers.length > 0">
        <h2 class="section-title">Workers</h2>
        <div class="table-container">
          <table class="data-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Hashrate</th>
                <th>Last Seen</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="worker in minerStore.workers" :key="worker.name">
                <td>{{ worker.name }}</td>
                <td>{{ formatHashrate(worker.hashrate) }}</td>
                <td>{{ formatTimeAgo(worker.last_seen) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Payments -->
      <div class="section">
        <h2 class="section-title">Recent Payments</h2>
        <div class="table-container">
          <table class="data-table">
            <thead>
              <tr>
                <th>Transaction</th>
                <th>Amount</th>
                <th>Time</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="payment in minerStore.payments" :key="payment.tx_hash">
                <td class="hash-cell">{{ shortenAddress(payment.tx_hash, 12) }}</td>
                <td>{{ formatTOS(payment.amount) }}</td>
                <td>{{ formatTime(payment.timestamp) }}</td>
                <td>
                  <span :class="['status-badge', payment.status]">{{ payment.status }}</span>
                </td>
              </tr>
              <tr v-if="minerStore.payments.length === 0">
                <td colspan="4" class="empty-row">No payments yet</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>

    <!-- Initial State -->
    <div v-else class="empty-state">
      <p>Enter a wallet address to view miner statistics</p>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMinerStore } from '../store/pool'
import { poolApi, formatHashrate, formatNumber, formatTOS, formatTime, formatTimeAgo, shortenAddress } from '../services/api'
import HashrateChart from '../components/HashrateChart.vue'

const route = useRoute()
const router = useRouter()
const minerStore = useMinerStore()
const searchAddress = ref('')
const hashrateHistory = ref([])

const lookupMiner = () => {
  if (searchAddress.value) {
    router.push(`/miners/${searchAddress.value}`)
  }
}

const fetchMinerHashrate = async (period) => {
  if (searchAddress.value) {
    return await poolApi.getMinerHashrateHistory(searchAddress.value, period)
  }
  return []
}

watch(() => route.params.address, async (address) => {
  if (address) {
    searchAddress.value = address
    await minerStore.fetchMiner(address)
    hashrateHistory.value = await poolApi.getMinerHashrateHistory(address, '24h')
  } else {
    minerStore.clear()
    hashrateHistory.value = []
  }
}, { immediate: true })

onMounted(() => {
  if (route.params.address) {
    searchAddress.value = route.params.address
  }
})
</script>
