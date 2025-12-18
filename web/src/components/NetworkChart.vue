<template>
  <div class="chart-container">
    <div class="chart-header">
      <h3 class="chart-title">{{ title }}</h3>
      <div class="chart-legend">
        <span class="legend-item" v-for="(dataset, index) in datasets" :key="index">
          <span class="legend-color" :style="{ background: dataset.color }"></span>
          {{ dataset.label }}
        </span>
      </div>
    </div>
    <div class="chart-wrapper">
      <Line :data="chartData" :options="chartOptions" />
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
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
    default: 'Network Stats'
  },
  datasets: {
    type: Array,
    default: () => [
      { label: 'Pool', data: [], color: '#6366f1' },
      { label: 'Network', data: [], color: '#10b981' }
    ]
  }
})

const chartData = computed(() => {
  const labels = props.datasets[0]?.data?.map(p => formatTimeLabel(p.timestamp)) || []

  return {
    labels,
    datasets: props.datasets.map(dataset => ({
      label: dataset.label,
      data: dataset.data.map(p => p.hashrate),
      borderColor: dataset.color,
      backgroundColor: 'transparent',
      borderWidth: 2,
      tension: 0.4,
      pointRadius: 0,
      pointHoverRadius: 4,
      pointHoverBackgroundColor: dataset.color,
      pointHoverBorderColor: '#fff',
      pointHoverBorderWidth: 2
    }))
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
      callbacks: {
        label: (context) => {
          return `${context.dataset.label}: ${formatHashrate(context.raw)}`
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

const formatTimeLabel = (timestamp) => {
  const date = new Date(timestamp * 1000)
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
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

.chart-legend {
  display: flex;
  gap: 1rem;
}

.legend-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.875rem;
  color: var(--text-secondary);
}

.legend-color {
  width: 12px;
  height: 12px;
  border-radius: 3px;
}

.chart-wrapper {
  height: 200px;
}

@media (max-width: 768px) {
  .chart-header {
    flex-direction: column;
    gap: 0.75rem;
    align-items: flex-start;
  }

  .chart-wrapper {
    height: 180px;
  }
}
</style>
