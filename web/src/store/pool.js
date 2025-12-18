import { defineStore } from 'pinia'
import { poolApi } from '../services/api'

export const usePoolStore = defineStore('pool', {
  state: () => ({
    stats: null,
    blocks: [],
    payments: [],
    loading: false,
    error: null,
    lastUpdate: null
  }),

  getters: {
    poolHashrate: (state) => state.stats?.pool?.hashrate || 0,
    networkHashrate: (state) => state.stats?.network?.hashrate || 0,
    minerCount: (state) => state.stats?.pool?.miners || 0,
    workerCount: (state) => state.stats?.pool?.workers || 0,
    blocksFound: (state) => state.stats?.pool?.blocks_found || 0,
    networkHeight: (state) => state.stats?.network?.height || 0,
    networkDifficulty: (state) => state.stats?.network?.difficulty || 0
  },

  actions: {
    async fetchStats() {
      this.loading = true
      this.error = null
      try {
        this.stats = await poolApi.getStats()
        this.lastUpdate = Date.now()
      } catch (err) {
        this.error = err.message
        console.error('Failed to fetch stats:', err)
      } finally {
        this.loading = false
      }
    },

    async fetchBlocks() {
      try {
        this.blocks = await poolApi.getBlocks()
      } catch (err) {
        console.error('Failed to fetch blocks:', err)
      }
    },

    async fetchPayments() {
      try {
        this.payments = await poolApi.getPayments()
      } catch (err) {
        console.error('Failed to fetch payments:', err)
      }
    },

    startAutoRefresh(interval = 10000) {
      this.fetchStats()
      return setInterval(() => this.fetchStats(), interval)
    }
  }
})

export const useMinerStore = defineStore('miner', {
  state: () => ({
    address: null,
    data: null,
    payments: [],
    loading: false,
    error: null
  }),

  getters: {
    hashrate: (state) => state.data?.hashrate || 0,
    balance: (state) => state.data?.balance || 0,
    immatureBalance: (state) => state.data?.immature || 0,
    totalPaid: (state) => state.data?.paid || 0,
    workers: (state) => state.data?.workers || []
  },

  actions: {
    async fetchMiner(address) {
      this.loading = true
      this.error = null
      this.address = address
      try {
        this.data = await poolApi.getMiner(address)
        this.payments = await poolApi.getMinerPayments(address)
      } catch (err) {
        this.error = err.response?.status === 404
          ? 'Miner not found'
          : err.message
        console.error('Failed to fetch miner:', err)
      } finally {
        this.loading = false
      }
    },

    clear() {
      this.address = null
      this.data = null
      this.payments = []
      this.error = null
    }
  }
})
