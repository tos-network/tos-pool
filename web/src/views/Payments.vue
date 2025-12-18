<template>
  <div class="payments">
    <h1 class="page-title">Recent Payments</h1>

    <!-- Stats Summary -->
    <div class="stats-grid small">
      <div class="stat-card">
        <div class="stat-content">
          <span class="stat-label">Total Paid</span>
          <span class="stat-value">{{ formatTOS(poolStore.stats?.pool?.total_paid) }}</span>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-content">
          <span class="stat-label">Payment Threshold</span>
          <span class="stat-value">0.1 TOS</span>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-content">
          <span class="stat-label">Payout Interval</span>
          <span class="stat-value">Every Hour</span>
        </div>
      </div>
    </div>

    <!-- Payments Table -->
    <div class="section">
      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>Transaction</th>
              <th>Recipient</th>
              <th>Amount</th>
              <th>Time</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="payment in poolStore.payments" :key="payment.tx_hash">
              <td class="hash-cell">
                <a :href="`https://explorer.tos.network/tx/${payment.tx_hash}`" target="_blank">
                  {{ shortenAddress(payment.tx_hash, 12) }}
                </a>
              </td>
              <td class="address-cell">
                <router-link :to="`/miners/${payment.address}`">
                  {{ shortenAddress(payment.address) }}
                </router-link>
              </td>
              <td>{{ formatTOS(payment.amount) }}</td>
              <td>{{ formatTime(payment.timestamp) }}</td>
              <td>
                <span :class="['status-badge', payment.status]">{{ payment.status }}</span>
              </td>
            </tr>
            <tr v-if="poolStore.payments.length === 0">
              <td colspan="5" class="empty-row">No payments yet</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Payment Info -->
    <div class="section">
      <h3 class="section-title">Payment Information</h3>
      <div class="info-box">
        <ul>
          <li><strong>Payment Scheme:</strong> PPLNS (Pay Per Last N Shares)</li>
          <li><strong>Minimum Payout:</strong> 0.1 TOS</li>
          <li><strong>Payout Frequency:</strong> Every hour</li>
          <li><strong>Pool Fee:</strong> 1%</li>
          <li><strong>Block Maturity:</strong> 100 confirmations</li>
        </ul>
      </div>
    </div>
  </div>
</template>

<script setup>
import { onMounted } from 'vue'
import { usePoolStore } from '../store/pool'
import { formatTOS, formatTime, shortenAddress } from '../services/api'

const poolStore = usePoolStore()

onMounted(async () => {
  await poolStore.fetchStats()
  await poolStore.fetchPayments()
})
</script>
