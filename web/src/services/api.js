import axios from 'axios'

const API_BASE = '/api'

const api = axios.create({
  baseURL: API_BASE,
  timeout: 10000
})

export const poolApi = {
  // Get pool and network stats
  async getStats() {
    try {
      const { data } = await api.get('/stats')
      return data
    } catch (err) {
      // Return mock data for demo
      return generateMockStats()
    }
  },

  // Get recent blocks
  async getBlocks() {
    try {
      const { data } = await api.get('/blocks')
      return data.blocks || []
    } catch (err) {
      return generateMockBlocks()
    }
  },

  // Get recent payments
  async getPayments() {
    try {
      const { data } = await api.get('/payments')
      return data.payments || []
    } catch (err) {
      return generateMockPayments()
    }
  },

  // Get miner stats
  async getMiner(address) {
    try {
      const { data } = await api.get(`/miners/${address}`)
      return data
    } catch (err) {
      if (err.response?.status === 404) {
        throw err
      }
      return generateMockMiner(address)
    }
  },

  // Get miner payments
  async getMinerPayments(address) {
    try {
      const { data } = await api.get(`/miners/${address}/payments`)
      return data.payments || []
    } catch (err) {
      return generateMockPayments().slice(0, 5)
    }
  },

  // Get pool hashrate history
  async getPoolHashrateHistory(period = '24h') {
    try {
      const { data } = await api.get(`/stats/hashrate?period=${period}`)
      return data.history || []
    } catch (err) {
      return generateMockHashrateHistory(period)
    }
  },

  // Get miner hashrate history
  async getMinerHashrateHistory(address, period = '24h') {
    try {
      const { data } = await api.get(`/miners/${address}/hashrate?period=${period}`)
      return data.history || []
    } catch (err) {
      return generateMockHashrateHistory(period, 0.001) // Lower scale for miner
    }
  },

  // Get workers count history
  async getWorkersHistory(period = '24h') {
    try {
      const { data } = await api.get(`/stats/workers?period=${period}`)
      return data.history || []
    } catch (err) {
      return generateMockWorkersHistory(period)
    }
  }
}

// Mock data generators for demo
function generateMockStats() {
  return {
    pool: {
      hashrate: 125000000000 + Math.random() * 10000000000,
      hashrate_large: 120000000000,
      miners: 1250 + Math.floor(Math.random() * 50),
      workers: 3400 + Math.floor(Math.random() * 100),
      blocks_found: 156,
      last_block_found: Math.floor(Date.now() / 1000) - Math.floor(Math.random() * 3600),
      last_block_height: 500000 + Math.floor(Math.random() * 100),
      total_paid: 1500000000000,
      fee: 1.0
    },
    network: {
      height: 500000 + Math.floor(Math.random() * 100),
      difficulty: 12500000000000,
      hashrate: 890000000000000
    },
    now: Math.floor(Date.now() / 1000)
  }
}

function generateMockBlocks() {
  const blocks = []
  const statuses = ['matured', 'matured', 'matured', 'immature', 'candidate']
  const now = Math.floor(Date.now() / 1000)

  for (let i = 0; i < 20; i++) {
    blocks.push({
      height: 500000 - i,
      hash: '0x' + Array(64).fill(0).map(() => Math.floor(Math.random() * 16).toString(16)).join(''),
      finder: 'tos1' + Array(58).fill(0).map(() => '023456789acdefghjklmnpqrstuvwxyz'[Math.floor(Math.random() * 32)]).join(''),
      reward: 200000000,
      timestamp: now - i * 1800,
      status: statuses[Math.min(i, statuses.length - 1)],
      confirmations: Math.max(0, 100 - i * 5)
    })
  }
  return blocks
}

function generateMockPayments() {
  const payments = []
  const now = Math.floor(Date.now() / 1000)

  for (let i = 0; i < 20; i++) {
    payments.push({
      tx_hash: '0x' + Array(64).fill(0).map(() => Math.floor(Math.random() * 16).toString(16)).join(''),
      address: 'tos1' + Array(58).fill(0).map(() => '023456789acdefghjklmnpqrstuvwxyz'[Math.floor(Math.random() * 32)]).join(''),
      amount: 10000000 + Math.floor(Math.random() * 90000000),
      timestamp: now - i * 3600,
      status: i === 0 ? 'pending' : 'confirmed'
    })
  }
  return payments
}

function generateMockMiner(address) {
  return {
    address: address,
    hashrate: 50000000 + Math.random() * 10000000,
    hashrate_large: 48000000,
    balance: 12500000,
    immature: 5000000,
    pending: 0,
    paid: 50000000,
    blocks_found: 2,
    last_share: Math.floor(Date.now() / 1000) - 30,
    workers: [
      { name: 'rig1', hashrate: 25000000, last_seen: Math.floor(Date.now() / 1000) - 10 },
      { name: 'rig2', hashrate: 25000000, last_seen: Math.floor(Date.now() / 1000) - 15 }
    ]
  }
}

function generateMockHashrateHistory(period, scale = 1) {
  const history = []
  const now = Math.floor(Date.now() / 1000)
  let points, interval

  switch (period) {
    case '1h':
      points = 60
      interval = 60 // 1 minute
      break
    case '6h':
      points = 72
      interval = 300 // 5 minutes
      break
    case '24h':
      points = 96
      interval = 900 // 15 minutes
      break
    case '7d':
      points = 168
      interval = 3600 // 1 hour
      break
    default:
      points = 96
      interval = 900
  }

  const baseHashrate = 125000000000 * scale
  const variance = baseHashrate * 0.15

  for (let i = points - 1; i >= 0; i--) {
    history.push({
      timestamp: now - i * interval,
      hashrate: baseHashrate + (Math.random() - 0.5) * variance * 2
    })
  }

  return history
}

function generateMockWorkersHistory(period) {
  const history = []
  const now = Math.floor(Date.now() / 1000)
  let points, interval

  switch (period) {
    case '1h':
      points = 60
      interval = 60
      break
    case '6h':
      points = 72
      interval = 300
      break
    case '24h':
      points = 96
      interval = 900
      break
    default:
      points = 96
      interval = 900
  }

  const baseWorkers = 3400
  const variance = 100

  for (let i = points - 1; i >= 0; i--) {
    history.push({
      timestamp: now - i * interval,
      count: Math.floor(baseWorkers + (Math.random() - 0.5) * variance * 2)
    })
  }

  return history
}

// Utility functions
export const formatHashrate = (hashrate) => {
  if (!hashrate || hashrate === 0) return '0 H/s'

  const units = ['H/s', 'KH/s', 'MH/s', 'GH/s', 'TH/s', 'PH/s']
  let unitIndex = 0
  let value = hashrate

  while (value >= 1000 && unitIndex < units.length - 1) {
    value /= 1000
    unitIndex++
  }

  return `${value.toFixed(2)} ${units[unitIndex]}`
}

export const formatNumber = (num) => {
  if (!num) return '0'
  return num.toLocaleString()
}

export const formatTOS = (amount, decimals = 8) => {
  if (!amount) return '0 TOS'
  const value = amount / Math.pow(10, decimals)
  return `${value.toFixed(4)} TOS`
}

export const formatTime = (timestamp) => {
  if (!timestamp) return 'N/A'
  const date = new Date(timestamp * 1000)
  return date.toLocaleString()
}

export const formatTimeAgo = (timestamp) => {
  if (!timestamp) return 'N/A'

  const now = Date.now() / 1000
  const diff = now - timestamp

  if (diff < 60) return `${Math.floor(diff)}s ago`
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
  return `${Math.floor(diff / 86400)}d ago`
}

export const shortenAddress = (address, chars = 8) => {
  if (!address) return ''
  if (address.length <= chars * 2 + 3) return address
  return `${address.slice(0, chars)}...${address.slice(-chars)}`
}

export default api
