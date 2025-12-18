<template>
  <div class="blocks">
    <h1 class="page-title">Blocks Found</h1>

    <!-- Stats Summary -->
    <div class="stats-grid small">
      <div class="stat-card">
        <div class="stat-content">
          <span class="stat-label">Total Blocks</span>
          <span class="stat-value">{{ formatNumber(poolStore.blocksFound) }}</span>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-content">
          <span class="stat-label">Last Block</span>
          <span class="stat-value">{{ lastBlockTime }}</span>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-content">
          <span class="stat-label">Pool Luck</span>
          <span class="stat-value">{{ poolLuck }}%</span>
        </div>
      </div>
    </div>

    <!-- Blocks Table -->
    <div class="section">
      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>Height</th>
              <th>Hash</th>
              <th>Finder</th>
              <th>Reward</th>
              <th>Confirmations</th>
              <th>Time</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="block in poolStore.blocks" :key="block.height">
              <td class="height-cell">{{ formatNumber(block.height) }}</td>
              <td class="hash-cell">
                <a :href="`https://explorer.tos.network/block/${block.hash}`" target="_blank">
                  {{ shortenAddress(block.hash, 12) }}
                </a>
              </td>
              <td class="address-cell">
                <router-link :to="`/miners/${block.finder}`">
                  {{ shortenAddress(block.finder) }}
                </router-link>
              </td>
              <td>{{ formatTOS(block.reward) }}</td>
              <td>{{ formatNumber(block.confirmations) }}</td>
              <td>{{ formatTime(block.timestamp) }}</td>
              <td>
                <span :class="['status-badge', block.status]">{{ block.status }}</span>
              </td>
            </tr>
            <tr v-if="poolStore.blocks.length === 0">
              <td colspan="7" class="empty-row">No blocks found yet</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Block Status Legend -->
    <div class="section">
      <h3 class="section-title">Block Status</h3>
      <div class="legend">
        <div class="legend-item">
          <span class="status-badge candidate">candidate</span>
          <span>Block found, waiting for confirmations</span>
        </div>
        <div class="legend-item">
          <span class="status-badge immature">immature</span>
          <span>Block confirmed, reward pending maturation</span>
        </div>
        <div class="legend-item">
          <span class="status-badge matured">matured</span>
          <span>Block fully confirmed, reward distributed</span>
        </div>
        <div class="legend-item">
          <span class="status-badge orphan">orphan</span>
          <span>Block orphaned, no reward</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted } from 'vue'
import { usePoolStore } from '../store/pool'
import { formatNumber, formatTOS, formatTime, formatTimeAgo, shortenAddress } from '../services/api'

const poolStore = usePoolStore()

const lastBlockTime = computed(() => {
  const timestamp = poolStore.stats?.pool?.last_block_found
  if (!timestamp) return 'N/A'
  return formatTimeAgo(timestamp)
})

const poolLuck = computed(() => {
  // Simplified luck calculation
  return '100'
})

onMounted(async () => {
  await poolStore.fetchStats()
  await poolStore.fetchBlocks()
})
</script>
