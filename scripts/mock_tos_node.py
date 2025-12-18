#!/usr/bin/env python3
"""
Mock TOS Node for Testing tos-pool

This script simulates a TOS blockchain node's JSON-RPC interface,
allowing tos-pool to run without a real node for development and testing.

Usage: python3 mock_tos_node.py [port]
Default port: 8545
"""

from http.server import HTTPServer, BaseHTTPRequestHandler
import json
import hashlib
import time
import sys


class MockTOSNode(BaseHTTPRequestHandler):
    """Mock TOS Node JSON-RPC handler"""

    height = 1000000
    difficulty = 1000000

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
        params = req.get('params', [])

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

    def handle_method(self, method, params):
        """Route RPC methods to handlers"""

        if method == 'tos_getWork':
            # Return [headerHash, seedHash, target, height]
            MockTOSNode.height += 1
            header = hashlib.sha256(f"header_{MockTOSNode.height}_{time.time()}".encode()).hexdigest()
            target = "0x" + "0" * 8 + "f" * 56  # Easy target for testing
            return [
                "0x" + header,
                "0x" + "0" * 64,
                target,
                hex(MockTOSNode.height)
            ]

        elif method == 'tos_getBlockTemplate':
            MockTOSNode.height += 1
            header = hashlib.sha256(f"template_{MockTOSNode.height}".encode()).hexdigest()
            return {
                "headerHash": "0x" + header,
                "parentHash": "0x" + "0" * 64,
                "height": MockTOSNode.height,
                "timestamp": int(time.time()),
                "difficulty": MockTOSNode.difficulty,
                "target": "0x" + "0" * 8 + "f" * 56
            }

        elif method == 'tos_blockNumber':
            return hex(MockTOSNode.height)

        elif method == 'tos_getBlockByNumber':
            block_num = params[0] if params else "latest"
            if block_num == "latest":
                num = MockTOSNode.height
            else:
                num = int(block_num, 16) if block_num.startswith("0x") else int(block_num)

            block_hash = hashlib.sha256(f"block_{num}".encode()).hexdigest()
            return {
                "hash": "0x" + block_hash,
                "parentHash": "0x" + hashlib.sha256(f"block_{num-1}".encode()).hexdigest(),
                "number": num,
                "timestamp": int(time.time()),
                "difficulty": MockTOSNode.difficulty,
                "totalDifficulty": hex(MockTOSNode.difficulty * num),
                "miner": "0x" + "0" * 40,
                "reward": 2000000000000000000,  # 2 TOS
                "size": 1000,
                "gasUsed": 21000,
                "gasLimit": 8000000,
                "transactionCount": 0
            }

        elif method == 'tos_getBlockByHash':
            return {
                "hash": params[0] if params else "0x" + "0" * 64,
                "number": MockTOSNode.height,
                "difficulty": MockTOSNode.difficulty,
                "timestamp": int(time.time()),
                "reward": 2000000000000000000
            }

        elif method == 'net_peerCount':
            return hex(10)  # 10 peers

        elif method == 'tos_syncing':
            return False  # Not syncing

        elif method == 'tos_gasPrice':
            return hex(1000000000)  # 1 Gwei

        elif method == 'tos_getBalance':
            return hex(100000000000000000000)  # 100 TOS

        elif method == 'tos_getTransactionCount':
            return hex(0)

        elif method == 'tos_estimateGas':
            return hex(21000)

        elif method == 'tos_sendTransaction':
            tx_hash = hashlib.sha256(f"tx_{time.time()}".encode()).hexdigest()
            print(f"[Node] Transaction sent: 0x{tx_hash[:16]}...")
            return "0x" + tx_hash

        elif method == 'tos_getTransactionReceipt':
            tx_hash = params[0] if params else ""
            return {
                "transactionHash": tx_hash,
                "blockHash": "0x" + hashlib.sha256(f"block_{MockTOSNode.height}".encode()).hexdigest(),
                "blockNumber": MockTOSNode.height,
                "status": 1,  # Success
                "gasUsed": 21000
            }

        elif method == 'tos_submitWork':
            print(f"[Node] *** BLOCK SUBMITTED! ***")
            print(f"[Node] Nonce: {params[0] if params else 'N/A'}")
            return True

        elif method == 'tos_submitBlock':
            print(f"[Node] *** BLOCK SUBMITTED! ***")
            return True

        elif method == 'net_version':
            return "1"  # Chain ID

        elif method == 'web3_clientVersion':
            return "MockTOSNode/1.0.0"

        else:
            print(f"[Node] Unknown method: {method}")
            return None


def main():
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8545

    server = HTTPServer(('127.0.0.1', port), MockTOSNode)

    print("=" * 60)
    print("Mock TOS Node for Testing")
    print("=" * 60)
    print(f"Listening on: http://127.0.0.1:{port}")
    print(f"Starting height: {MockTOSNode.height}")
    print(f"Difficulty: {MockTOSNode.difficulty}")
    print("")
    print("Supported methods:")
    print("  - tos_getWork, tos_getBlockTemplate")
    print("  - tos_blockNumber, tos_getBlockByNumber, tos_getBlockByHash")
    print("  - tos_submitWork, tos_submitBlock")
    print("  - net_peerCount, tos_syncing, tos_gasPrice")
    print("  - tos_getBalance, tos_sendTransaction, tos_getTransactionReceipt")
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
