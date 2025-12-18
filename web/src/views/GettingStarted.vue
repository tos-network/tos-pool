<template>
  <div class="getting-started">
    <h1 class="page-title">Getting Started</h1>

    <!-- Quick Start -->
    <div class="section">
      <h2 class="section-title">Quick Start</h2>
      <div class="steps">
        <div class="step">
          <div class="step-number">1</div>
          <div class="step-content">
            <h3>Get a TOS Wallet</h3>
            <p>Create a TOS wallet address to receive your mining rewards.</p>
            <a href="https://tos.network/wallet" target="_blank" class="btn btn-outline">
              Create Wallet
            </a>
          </div>
        </div>

        <div class="step">
          <div class="step-number">2</div>
          <div class="step-content">
            <h3>Download tosminer</h3>
            <p>Download the latest version of tosminer for your operating system.</p>
            <a href="https://github.com/tos-network/tosminer/releases" target="_blank" class="btn btn-outline">
              Download
            </a>
          </div>
        </div>

        <div class="step">
          <div class="step-number">3</div>
          <div class="step-content">
            <h3>Start Mining</h3>
            <p>Run tosminer with your wallet address and start earning TOS!</p>
          </div>
        </div>
      </div>
    </div>

    <!-- Connection Details -->
    <div class="section">
      <h2 class="section-title">Pool Connection</h2>
      <div class="connection-grid">
        <div class="connection-card">
          <h3>Stratum (TCP)</h3>
          <code>stratum+tcp://pool.tos.network:3333</code>
        </div>
        <div class="connection-card">
          <h3>Stratum (TLS/SSL)</h3>
          <code>stratum+ssl://pool.tos.network:3334</code>
        </div>
      </div>
    </div>

    <!-- Mining Commands -->
    <div class="section">
      <h2 class="section-title">Mining Commands</h2>

      <div class="command-block">
        <h4>NVIDIA GPU (CUDA)</h4>
        <pre><code>tosminer -U -P stratum+tcp://pool.tos.network:3333 -u YOUR_TOS_ADDRESS.rig1</code></pre>
      </div>

      <div class="command-block">
        <h4>AMD GPU (OpenCL)</h4>
        <pre><code>tosminer -G -P stratum+tcp://pool.tos.network:3333 -u YOUR_TOS_ADDRESS.rig1</code></pre>
      </div>

      <div class="command-block">
        <h4>With TLS Encryption</h4>
        <pre><code>tosminer -U -P stratum+ssl://pool.tos.network:3334 -u YOUR_TOS_ADDRESS.rig1</code></pre>
      </div>

      <div class="command-block">
        <h4>Multiple GPUs</h4>
        <pre><code>tosminer -U --cuda-devices 0,1,2,3 -P stratum+tcp://pool.tos.network:3333 -u YOUR_TOS_ADDRESS.rig1</code></pre>
      </div>
    </div>

    <!-- Command Options -->
    <div class="section">
      <h2 class="section-title">Command Options</h2>
      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>Option</th>
              <th>Description</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td><code>-U, --cuda</code></td>
              <td>Use CUDA mining (NVIDIA GPUs)</td>
            </tr>
            <tr>
              <td><code>-G, --opencl</code></td>
              <td>Use OpenCL mining (AMD/Intel GPUs)</td>
            </tr>
            <tr>
              <td><code>-P, --pool URL</code></td>
              <td>Pool connection URL</td>
            </tr>
            <tr>
              <td><code>-u, --user</code></td>
              <td>Wallet address and worker name (address.worker)</td>
            </tr>
            <tr>
              <td><code>--cuda-devices</code></td>
              <td>Specify CUDA device indices (e.g., 0,1,2)</td>
            </tr>
            <tr>
              <td><code>--api-port</code></td>
              <td>Enable HTTP API on specified port</td>
            </tr>
            <tr>
              <td><code>-M, --benchmark</code></td>
              <td>Run benchmark mode</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Pool Info -->
    <div class="section">
      <h2 class="section-title">Pool Information</h2>
      <div class="info-grid">
        <div class="info-item">
          <span class="info-label">Algorithm</span>
          <span class="info-value">TOS Hash V3</span>
        </div>
        <div class="info-item">
          <span class="info-label">Payment Scheme</span>
          <span class="info-value">PPLNS</span>
        </div>
        <div class="info-item">
          <span class="info-label">Pool Fee</span>
          <span class="info-value">1%</span>
        </div>
        <div class="info-item">
          <span class="info-label">Minimum Payout</span>
          <span class="info-value">0.1 TOS</span>
        </div>
        <div class="info-item">
          <span class="info-label">Payout Frequency</span>
          <span class="info-value">Hourly</span>
        </div>
        <div class="info-item">
          <span class="info-label">Block Maturity</span>
          <span class="info-value">100 confirmations</span>
        </div>
      </div>
    </div>

    <!-- FAQ -->
    <div class="section">
      <h2 class="section-title">FAQ</h2>
      <div class="faq-list">
        <div class="faq-item">
          <h4>What hashrate can I expect?</h4>
          <p>Performance varies by GPU. RTX 4090: ~500-1000 KH/s, RTX 3080: ~200-400 KH/s. Run <code>tosminer --benchmark</code> to test your hardware.</p>
        </div>
        <div class="faq-item">
          <h4>Why is my hashrate fluctuating?</h4>
          <p>The miner uses Exponential Moving Average (EMA) with a 30-second window. Initial readings may vary until enough samples are collected.</p>
        </div>
        <div class="faq-item">
          <h4>How do I set a custom difficulty?</h4>
          <p>The pool uses variable difficulty (vardiff) that automatically adjusts to your hashrate. No manual configuration needed.</p>
        </div>
        <div class="faq-item">
          <h4>When will I receive payment?</h4>
          <p>Payments are processed hourly. Your balance must be above 0.1 TOS to receive a payout.</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
</script>
