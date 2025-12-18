#!/usr/bin/env python3
"""
Mock TOS Node for Testing tos-pool

This script simulates a TOS blockchain node's JSON-RPC interface,
allowing tos-pool to run without a real node for development and testing.

The API is aligned with the actual TOS daemon JSON-RPC interface as defined
in the TOS source code (daemon/src/rpc/rpc.rs).

Usage: python3 mock_tos_node.py [port]
Default port: 8545
"""

from http.server import HTTPServer, BaseHTTPRequestHandler
import json
import hashlib
import time
import sys
import secrets


class MockTOSNode(BaseHTTPRequestHandler):
    """Mock TOS Node JSON-RPC handler aligned with TOS daemon API"""

    height = 1000000
    topoheight = 1000000
    difficulty = "1000000"  # TOS uses string difficulty
    # TOS native asset hash (64 zeros)
    NATIVE_ASSET = "0000000000000000000000000000000000000000000000000000000000000000"

    def log_message(self, format, *args):
        """Custom log format"""
        print(f"[Node] {time.strftime('%H:%M:%S')} - {args[0]}")

    def do_POST(self):
        """Handle JSON-RPC POST requests"""
        content_length = int(self.headers['Content-Length'])
        body = self.rfile.read(content_length)

        try:
            req = json.loads(body)
        except json.JSONDecodeError:
            self.send_error(400, "Invalid JSON")
            return

        method = req.get('method', '')
        req_id = req.get('id', 1)
        params = req.get('params', {})  # TOS daemon uses object params, not array

        result = self.handle_method(method, params)

        response = {
            "jsonrpc": "2.0",
            "id": req_id,
            "result": result
        }

        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        self.wfile.write(json.dumps(response).encode())

    def _generate_block(self, topo, include_txs=False):
        """Generate a mock block response matching TOS daemon RPCBlockResponse"""
        block_hash = hashlib.sha256(f"block_{topo}".encode()).hexdigest()
        parent_hash = hashlib.sha256(f"block_{topo-1}".encode()).hexdigest()

        block = {
            "hash": block_hash,
            "topoheight": topo,
            "block_type": "Normal",  # "Sync", "Side", "Orphaned", "Normal"
            "difficulty": MockTOSNode.difficulty,
            "supply": 100000000000000,  # Total supply in atomic units
            "reward": 100000000,  # Block reward in atomic units
            "miner_reward": 90000000,  # 90% to miner
            "dev_reward": 10000000,  # 10% to dev fund
            "cumulative_difficulty": str(int(MockTOSNode.difficulty) * topo),
            "total_fees": 0,
            "total_size_in_bytes": 1024,
            "version": 1,  # BlockVersion for hard fork
            "tips": [parent_hash],  # Array of parent hashes (DAG structure)
            "timestamp": int(time.time() * 1000),  # TOS uses milliseconds
            "height": topo,  # In DAG, height can differ from topoheight
            "nonce": secrets.randbelow(2**64),
            "extra_nonce": "0" * 64,  # Hex encoded
            "miner": "tos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqhqsrry",
            "txs_hashes": [],
        }

        if include_txs:
            block["transactions"] = []

        return block

    def handle_method(self, method, params):
        """Route RPC methods to handlers - aligned with TOS daemon API"""

        # ========== Mining Methods ==========

        if method == 'get_block_template':
            # TOS daemon get_block_template (rpc.rs:730)
            # Request: { "address": "tos1..." }
            # Response: GetBlockTemplateResult
            MockTOSNode.height += 1
            MockTOSNode.topoheight += 1

            # Generate a mock block template (hex string)
            template_data = f"template_{MockTOSNode.height}_{time.time()}"
            template = hashlib.sha256(template_data.encode()).hexdigest()

            return {
                "template": template,
                "algorithm": "tos/v3",  # POW algorithm (V1, V2, V3 - currently V3)
                "height": MockTOSNode.height,
                "topoheight": MockTOSNode.topoheight,
                "difficulty": MockTOSNode.difficulty
            }

        elif method == 'submit_block':
            # TOS daemon submit_block (rpc.rs:817)
            # Request: { "block_template": "hex", "miner_work": "hex" (optional) }
            # Response: bool
            print(f"[Node] *** BLOCK SUBMITTED! ***")
            block_template = params.get('block_template', 'N/A')
            miner_work = params.get('miner_work')
            print(f"[Node] Template: {block_template[:32]}..." if len(str(block_template)) > 32 else f"[Node] Template: {block_template}")
            if miner_work:
                print(f"[Node] Miner work: {miner_work[:32]}..." if len(str(miner_work)) > 32 else f"[Node] Miner work: {miner_work}")
            return True

        elif method == 'get_miner_work':
            # TOS daemon get_miner_work (rpc.rs:769)
            # Request: { "template": "hex", "address": "tos1..." (optional) }
            # Response: GetMinerWorkResult
            template = params.get('template', '')
            miner_work = hashlib.sha256(f"miner_work_{template}".encode()).hexdigest()

            return {
                "algorithm": "tos/v3",
                "miner_work": miner_work,
                "height": MockTOSNode.height,
                "difficulty": MockTOSNode.difficulty,
                "topoheight": MockTOSNode.topoheight
            }

        # ========== Block Query Methods ==========

        elif method == 'get_topoheight':
            # TOS daemon get_topoheight (rpc.rs:638)
            # Response: TopoHeight (u64)
            return MockTOSNode.topoheight

        elif method == 'get_height':
            # TOS daemon get_height (rpc.rs:632)
            # Response: u64
            return MockTOSNode.height

        elif method == 'get_stable_height':
            # TOS daemon get_stable_height (rpc.rs:663)
            # Response: u64
            return MockTOSNode.height - 8

        elif method == 'get_stable_topoheight':
            # TOS daemon get_stable_topoheight (rpc.rs:672)
            # Response: TopoHeight
            return MockTOSNode.topoheight - 8

        elif method == 'get_block_at_topoheight':
            # TOS daemon get_block_at_topoheight (rpc.rs:692)
            # Request: { "topoheight": N, "include_txs": bool }
            # Response: RPCBlockResponse
            topo = params.get('topoheight', MockTOSNode.topoheight)
            include_txs = params.get('include_txs', False)
            return self._generate_block(topo, include_txs)

        elif method == 'get_block_by_hash':
            # TOS daemon get_block_by_hash (rpc.rs:706)
            # Request: { "hash": "Hash", "include_txs": bool }
            # Response: RPCBlockResponse
            block_hash = params.get('hash', "0" * 64)
            include_txs = params.get('include_txs', False)
            block = self._generate_block(MockTOSNode.topoheight, include_txs)
            block["hash"] = block_hash  # Use the requested hash
            return block

        elif method == 'get_top_block':
            # TOS daemon get_top_block (rpc.rs:716)
            # Request: { "include_txs": bool }
            # Response: RPCBlockResponse
            include_txs = params.get('include_txs', False)
            return self._generate_block(MockTOSNode.topoheight, include_txs)

        elif method == 'get_blocks_at_height':
            # Request: { "height": N, "include_txs": bool }
            # Returns array of blocks at the given height (DAG can have multiple)
            height = params.get('height', MockTOSNode.height)
            include_txs = params.get('include_txs', False)
            return [self._generate_block(height, include_txs)]

        # ========== Network Methods ==========

        elif method == 'p2p_status':
            # TOS daemon p2p_status (rpc.rs:1310)
            # Response: P2pStatusResult
            return {
                "peer_count": 10,
                "max_peers": 32,
                "tag": None,
                "our_topoheight": MockTOSNode.topoheight,
                "best_topoheight": MockTOSNode.topoheight,
                "median_topoheight": MockTOSNode.topoheight,
                "peer_id": secrets.randbelow(2**64)
            }

        elif method == 'get_info':
            # TOS daemon get_info (rpc.rs:948)
            # Response: GetInfoResult
            return {
                "height": MockTOSNode.height,
                "topoheight": MockTOSNode.topoheight,
                "stableheight": MockTOSNode.height - 8,
                "stable_topoheight": MockTOSNode.topoheight - 8,
                "pruned_topoheight": None,
                "top_block_hash": hashlib.sha256(f"block_{MockTOSNode.topoheight}".encode()).hexdigest(),
                "circulating_supply": 100000000000000,
                "burned_supply": 0,
                "emitted_supply": 100000000000000,
                "maximum_supply": 1000000000000000,
                "difficulty": MockTOSNode.difficulty,
                "block_time_target": 15000,  # 15 seconds in ms
                "average_block_time": 15000,
                "block_reward": 100000000,
                "dev_reward": 10000000,
                "miner_reward": 90000000,
                "mempool_size": 0,
                "version": "1.0.0",
                "network": "mainnet",
                "block_version": 1
            }

        elif method == 'get_version':
            # TOS daemon get_version (rpc.rs:627)
            # Response: String
            return "MockTOSNode/1.0.0"

        elif method == 'get_difficulty':
            # TOS daemon get_difficulty
            # Response: Difficulty
            return MockTOSNode.difficulty

        elif method == 'get_tips':
            # TOS daemon get_tips
            # Response: IndexSet<Hash>
            tip_hash = hashlib.sha256(f"block_{MockTOSNode.topoheight}".encode()).hexdigest()
            return [tip_hash]

        elif method == 'get_peers':
            # TOS daemon get_peers (rpc.rs:1339)
            # Response: GetPeersResponse
            return {
                "peers": [],
                "total_peers": 10,
                "hidden_peers": 0
            }

        # ========== Balance/Account Methods ==========

        elif method == 'get_balance':
            # TOS daemon get_balance (rpc.rs:841)
            # Request: { "address": "tos1...", "asset": "0000...0000" }
            # Response: GetBalanceResult
            address = params.get('address', '')
            asset = params.get('asset', MockTOSNode.NATIVE_ASSET)

            return {
                "balance": 100000000000000,  # 100000 TOS (assuming 9 decimals)
                "topoheight": MockTOSNode.topoheight
            }

        elif method == 'get_balance_at_topoheight':
            # TOS daemon get_balance_at_topoheight (rpc.rs:1020)
            # Request: { "address": "tos1...", "asset": "Hash", "topoheight": N }
            # Response: VersionedBalance
            return {
                "previous_topoheight": MockTOSNode.topoheight - 1,
                "output_balance": None,
                "final_balance": 100000000000000,
                "balance_type": "input"
            }

        elif method == 'has_balance':
            # TOS daemon has_balance (rpc.rs:919)
            # Request: { "address": "tos1...", "asset": "Hash", "topoheight": N (optional) }
            # Response: HasBalanceResult
            return {
                "exist": True
            }

        elif method == 'get_nonce':
            # TOS daemon get_nonce (rpc.rs:1075)
            # Request: { "address": "tos1..." }
            # Response: GetNonceResult
            return {
                "topoheight": MockTOSNode.topoheight,
                "nonce": 0,
                "previous_topoheight": None
            }

        elif method == 'get_nonce_at_topoheight':
            # TOS daemon get_nonce_at_topoheight (rpc.rs:1096)
            # Request: { "address": "tos1...", "topoheight": N }
            # Response: VersionedNonce
            return {
                "nonce": 0,
                "previous_topoheight": None
            }

        elif method == 'has_nonce':
            # TOS daemon has_nonce (rpc.rs:1050)
            # Request: { "address": "tos1...", "topoheight": N (optional) }
            # Response: HasNonceResult
            return {
                "exist": True
            }

        # ========== Transaction Methods ==========

        elif method == 'submit_transaction':
            # TOS daemon submit_transaction (rpc.rs:1255)
            # Request: { "data": "hex_encoded_tx" }
            # Response: bool
            tx_data = params.get('data', '')
            tx_hash = hashlib.sha256(f"tx_{time.time()}_{tx_data}".encode()).hexdigest()
            print(f"[Node] Transaction submitted: {tx_hash[:16]}...")
            return True

        elif method == 'get_transaction':
            # TOS daemon get_transaction (rpc.rs:1277)
            # Request: { "hash": "Hash" }
            # Response: TransactionResponse
            tx_hash = params.get('hash', "0" * 64)
            block_hash = hashlib.sha256(f"block_{MockTOSNode.topoheight}".encode()).hexdigest()

            return {
                "hash": tx_hash,
                "blocks": [block_hash],
                "executed_in_block": block_hash,
                "in_mempool": False,
                "first_seen": int(time.time()),  # Unix seconds
                "version": 0,
                "source": "tos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqhqsrry",
                "data": {
                    "Transfer": [{
                        "asset": MockTOSNode.NATIVE_ASSET,
                        "amount": 1000000000,
                        "destination": "tos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqhqsrry"
                    }]
                },
                "fee": 1000,
                "nonce": 0,
                "reference": {
                    "topoheight": MockTOSNode.topoheight - 1,
                    "hash": hashlib.sha256(f"block_{MockTOSNode.topoheight - 1}".encode()).hexdigest()
                },
                "multisig": None,
                "signature": "0" * 128,
                "size": 256
            }

        elif method == 'get_transactions':
            # TOS daemon get_transactions (rpc.rs:1623)
            # Request: { "tx_hashes": ["Hash", ...] }
            # Response: Vec<TransactionResponse>
            tx_hashes = params.get('tx_hashes', [])
            transactions = []
            for tx_hash in tx_hashes:
                # Reuse get_transaction logic
                tx = self.handle_method('get_transaction', {'hash': tx_hash})
                transactions.append(tx)
            return transactions

        elif method == 'get_transaction_executor':
            # TOS daemon get_transaction_executor (rpc.rs:1289)
            # Request: { "hash": "Hash" }
            # Response: GetTransactionExecutorResult
            block_hash = hashlib.sha256(f"block_{MockTOSNode.topoheight}".encode()).hexdigest()
            return {
                "block_topoheight": MockTOSNode.topoheight,
                "block_timestamp": int(time.time() * 1000),  # milliseconds
                "block_hash": block_hash
            }

        elif method == 'get_mempool':
            # TOS daemon get_mempool (rpc.rs:1365)
            # Request: { "maximum": N (optional), "skip": N (optional) }
            # Response: GetMempoolResult
            return {
                "transactions": [],
                "total": 0
            }

        # ========== Asset Methods ==========

        elif method == 'get_asset':
            # Get asset info
            # Request: { "asset": "Hash" }
            asset = params.get('asset', MockTOSNode.NATIVE_ASSET)
            return {
                "topoheight": 0,
                "decimals": 9
            }

        # ========== DAG-specific Methods ==========

        elif method == 'get_dag_order':
            # Request: { "start_topoheight": N, "end_topoheight": N }
            start = params.get('start_topoheight', MockTOSNode.topoheight - 10)
            end = params.get('end_topoheight', MockTOSNode.topoheight)

            hashes = []
            for i in range(start, end + 1):
                hashes.append(hashlib.sha256(f"block_{i}".encode()).hexdigest())
            return hashes

        elif method == 'is_tx_executed_in_block':
            # Check if tx was executed in a specific block
            # Request: { "tx_hash": "Hash", "block_hash": "Hash" }
            return True

        elif method == 'get_block_count':
            # Get total block count
            return MockTOSNode.topoheight

        # ========== Unknown Method ==========

        else:
            print(f"[Node] Unknown method: {method}")
            return None


def main():
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8545

    server = HTTPServer(('127.0.0.1', port), MockTOSNode)

    print("=" * 60)
    print("Mock TOS Node for Testing (TOS Daemon API)")
    print("=" * 60)
    print(f"Listening on: http://127.0.0.1:{port}")
    print(f"Starting height: {MockTOSNode.height}")
    print(f"Starting topoheight: {MockTOSNode.topoheight}")
    print(f"Difficulty: {MockTOSNode.difficulty}")
    print("")
    print("Mining methods:")
    print("  - get_block_template, submit_block, get_miner_work")
    print("")
    print("Block query methods:")
    print("  - get_topoheight, get_height, get_stable_height, get_stable_topoheight")
    print("  - get_block_at_topoheight, get_block_by_hash, get_top_block")
    print("  - get_blocks_at_height, get_tips, get_dag_order")
    print("")
    print("Network methods:")
    print("  - p2p_status, get_info, get_version, get_difficulty, get_peers")
    print("")
    print("Account methods:")
    print("  - get_balance, get_balance_at_topoheight, has_balance")
    print("  - get_nonce, get_nonce_at_topoheight, has_nonce")
    print("")
    print("Transaction methods:")
    print("  - submit_transaction, get_transaction, get_transactions")
    print("  - get_transaction_executor, get_mempool")
    print("")
    print("Press Ctrl+C to stop")
    print("=" * 60)
    print("")

    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\n[Node] Shutting down...")
        server.shutdown()


if __name__ == '__main__':
    main()
