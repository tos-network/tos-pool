<template>
  <div class="chart-container">
    <div class="chart-header">
      <h3 class="chart-title">{{ title }}</h3>
      <div class="chart-controls">
        <button
          v-for="period in periods"
          :key="period.value"
          :class="['period-btn', { active: selectedPeriod === period.value }]"
          @click="selectPeriod(period.value)"
        >
          {{ period.label }}
        </button>
      </div>
    </div>
    <div class="chart-wrapper">
      <Line :data="chartData" :options="chartOptions" />
    </div>
    <div class="chart-stats">
      <div class="chart-stat">
        <span class="stat-label">Current</span>
        <span class="stat-value">{{ formatHashrate(currentHashrate) }}</span>
      </div>
      <div class="chart-stat">
        <span class="stat-label">Average</span>
        <span class="stat-value">{{ formatHashrate(averageHashrate) }}</span>
      </div>
      <div class="chart-stat">
        <span class="stat-label">Peak</span>
        <span class="stat-value">{{ formatHashrate(peakHashrate) }}</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { Line } from 'vue-chartjs'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { formatHashrate } from '../services/api'

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
)

const props = defineProps({
  title: {
    type: String,
    default: 'Hashrate'
  },
  data: {
    type: Array,
    default: () => []
  },
  color: {
    type: String,
    default: '#6366f1'
  },
  fetchData: {
    type: Function,
    default: null
  }
})

const periods = [
  { label: '1H', value: '1h' },
  { label: '6H', value: '6h' },
  { label: '24H', value: '24h' },
  { label: '7D', value: '7d' }
]

const selectedPeriod = ref('24h')
const chartDataPoints = ref([])
let refreshInterval = null

const selectPeriod = async (period) => {
  selectedPeriod.value = period
  if (props.fetchData) {
    chartDataPoints.value = await props.fetchData(period)
  }
}

const chartData = computed(() => {
  const points = chartDataPoints.value.length > 0 ? chartDataPoints.value : props.data

  return {
    labels: points.map(p => formatTimeLabel(p.timestamp)),
    datasets: [
      {
        label: 'Hashrate',
        data: points.map(p => p.hashrate),
        borderColor: props.color,
        backgroundColor: `${props.color}20`,
        borderWidth: 2,
        fill: true,
        tension: 0.4,
        pointRadius: 0,
        pointHoverRadius: 4,
        pointHoverBackgroundColor: props.color,
        pointHoverBorderColor: '#fff',
        pointHoverBorderWidth: 2
      }
    ]
  }
})

const chartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  interaction: {
    intersect: false,
    mode: 'index'
  },
  plugins: {
    legend: {
      display: false
    },
    tooltip: {
      backgroundColor: '#1e293b',
      titleColor: '#f1f5f9',
      bodyColor: '#94a3b8',
      borderColor: '#334155',
      borderWidth: 1,
      padding: 12,
      displayColors: false,
      callbacks: {
        title: (context) => {
          const index = context[0].dataIndex
          const points = chartDataPoints.value.length > 0 ? chartDataPoints.value : props.data
          if (points[index]) {
            return new Date(points[index].timestamp * 1000).toLocaleString()
          }
          return ''
        },
        label: (context) => {
          return `Hashrate: ${formatHashrate(context.raw)}`
        }
      }
    }
  },
  scales: {
    x: {
      grid: {
        color: '#334155',
        drawBorder: false
      },
      ticks: {
        color: '#64748b',
        maxTicksLimit: 8,
        font: {
          size: 11
        }
      }
    },
    y: {
      grid: {
        color: '#334155',
        drawBorder: false
      },
      ticks: {
        color: '#64748b',
        font: {
          size: 11
        },
        callback: (value) => formatHashrateShort(value)
      },
      beginAtZero: true
    }
  }
}

const currentHashrate = computed(() => {
  const points = chartDataPoints.value.length > 0 ? chartDataPoints.value : props.data
  if (points.length === 0) return 0
  return points[points.length - 1].hashrate
})

const averageHashrate = computed(() => {
  const points = chartDataPoints.value.length > 0 ? chartDataPoints.value : props.data
  if (points.length === 0) return 0
  const sum = points.reduce((acc, p) => acc + p.hashrate, 0)
  return sum / points.length
})

const peakHashrate = computed(() => {
  const points = chartDataPoints.value.length > 0 ? chartDataPoints.value : props.data
  if (points.length === 0) return 0
  return Math.max(...points.map(p => p.hashrate))
})

const formatTimeLabel = (timestamp) => {
  const date = new Date(timestamp * 1000)
  const period = selectedPeriod.value

  if (period === '1h' || period === '6h') {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  } else if (period === '24h') {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  } else {
    return date.toLocaleDateString([], { month: 'short', day: 'numeric' })
  }
}

const formatHashrateShort = (value) => {
  if (value === 0) return '0'
  const units = ['', 'K', 'M', 'G', 'T', 'P']
  let unitIndex = 0
  while (value >= 1000 && unitIndex < units.length - 1) {
    value /= 1000
    unitIndex++
  }
  return `${value.toFixed(1)}${units[unitIndex]}`
}

onMounted(async () => {
  if (props.fetchData) {
    chartDataPoints.value = await props.fetchData(selectedPeriod.value)

    // Auto-refresh every 30 seconds
    refreshInterval = setInterval(async () => {
      chartDataPoints.value = await props.fetchData(selectedPeriod.value)
    }, 30000)
  }
})

onUnmounted(() => {
  if (refreshInterval) {
    clearInterval(refreshInterval)
  }
})
</script>

<style scoped>
.chart-container {
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  border-radius: 12px;
  padding: 1.5rem;
}

.chart-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.chart-title {
  font-size: 1rem;
  font-weight: 600;
  color: var(--text-primary);
}

.chart-controls {
  display: flex;
  gap: 0.25rem;
  background: var(--bg-tertiary);
  padding: 0.25rem;
  border-radius: 8px;
}

.period-btn {
  padding: 0.375rem 0.75rem;
  background: transparent;
  border: none;
  border-radius: 6px;
  color: var(--text-secondary);
  font-size: 0.75rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
}

.period-btn:hover {
  color: var(--text-primary);
}

.period-btn.active {
  background: var(--primary);
  color: white;
}

.chart-wrapper {
  height: 250px;
  margin-bottom: 1rem;
}

.chart-stats {
  display: flex;
  justify-content: space-around;
  padding-top: 1rem;
  border-top: 1px solid var(--border-color);
}

.chart-stat {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.25rem;
}

.chart-stat .stat-label {
  font-size: 0.75rem;
  color: var(--text-muted);
  text-transform: uppercase;
}

.chart-stat .stat-value {
  font-size: 1rem;
  font-weight: 600;
  color: var(--text-primary);
}

@media (max-width: 768px) {
  .chart-header {
    flex-direction: column;
    gap: 1rem;
    align-items: flex-start;
  }

  .chart-wrapper {
    height: 200px;
  }

  .chart-stats {
    flex-wrap: wrap;
    gap: 1rem;
  }
}
</style>
