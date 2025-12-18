#!/usr/bin/env python3
"""
Stratum Protocol Compatibility Test

Simulates tosminer connecting to tos-pool to verify protocol compatibility.
"""

import socket
import json
import sys
import time

def send_json(sock, obj):
    """Send JSON-RPC message"""
    msg = json.dumps(obj) + '\n'
    print(f">>> {msg.strip()}")
    sock.sendall(msg.encode())

def recv_json(sock, timeout=5):
    """Receive JSON-RPC message"""
    sock.settimeout(timeout)
    try:
        data = b''
        while b'\n' not in data:
            chunk = sock.recv(4096)
            if not chunk:
                return None
            data += chunk

        line = data.split(b'\n')[0].decode()
        print(f"<<< {line}")
        return json.loads(line)
    except socket.timeout:
        return None

def test_stratum_protocol(host='127.0.0.1', port=3333):
    """Test stratum protocol compatibility"""
    print(f"\n{'='*60}")
    print(f"Testing Stratum Protocol Compatibility")
    print(f"Connecting to {host}:{port}")
    print(f"{'='*60}\n")

    # Connect
    try:
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.connect((host, port))
        print(f"[OK] Connected to {host}:{port}\n")
    except Exception as e:
        print(f"[FAIL] Connection failed: {e}")
        return False

    try:
        # 1. Test mining.subscribe
        print("--- Test 1: mining.subscribe ---")
        send_json(sock, {
            "id": 1,
            "method": "mining.subscribe",
            "params": ["tosminer/1.0.0"]
        })

        resp = recv_json(sock)
        if not resp:
            print("[FAIL] No response to subscribe")
            return False

        if resp.get('error'):
            print(f"[FAIL] Subscribe error: {resp['error']}")
            return False

        result = resp.get('result', [])
        if len(result) >= 3:
            extranonce1 = result[1]
            extranonce2_size = result[2]
            print(f"[OK] Subscribed: extranonce1={extranonce1}, extranonce2_size={extranonce2_size}\n")
        else:
            print(f"[WARN] Unexpected subscribe result format: {result}\n")

        # Receive set_difficulty notification
        diff_resp = recv_json(sock, timeout=2)
        if diff_resp and diff_resp.get('method') == 'mining.set_difficulty':
            difficulty = diff_resp['params'][0]
            print(f"[OK] Received difficulty: {difficulty}\n")

        # 2. Test mining.authorize
        print("--- Test 2: mining.authorize ---")
        test_address = "tos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"
        send_json(sock, {
            "id": 2,
            "method": "mining.authorize",
            "params": [f"{test_address}.test_worker", "x"]
        })

        resp = recv_json(sock)
        if not resp:
            print("[FAIL] No response to authorize")
            return False

        if resp.get('error'):
            print(f"[WARN] Authorize error (expected with invalid address): {resp['error']}")
        elif resp.get('result') == True:
            print(f"[OK] Authorized successfully\n")
        else:
            print(f"[INFO] Authorize result: {resp}\n")

        # Check if we receive mining.notify (job)
        job_resp = recv_json(sock, timeout=2)
        if job_resp and job_resp.get('method') == 'mining.notify':
            params = job_resp.get('params', [])
            print(f"[OK] Received job notification:")
            print(f"     Job ID: {params[0] if len(params) > 0 else 'N/A'}")
            print(f"     Header: {params[1][:32] if len(params) > 1 else 'N/A'}...")
            print(f"     Target: {params[2][:32] if len(params) > 2 else 'N/A'}...")
            print(f"     Height: {params[3] if len(params) > 3 else 'N/A'}")
            print(f"     Clean:  {params[4] if len(params) > 4 else 'N/A'}\n")

            # Verify format matches tosminer expectations
            if len(params) == 5:
                print("[OK] Job format matches tosminer expectations (5 params)")
            else:
                print(f"[WARN] Job format has {len(params)} params, expected 5")

        # 3. Test mining.submit (simulate share submission)
        print("\n--- Test 3: mining.submit (4-param format) ---")
        send_json(sock, {
            "id": 3,
            "method": "mining.submit",
            "params": [
                f"{test_address}.test_worker",
                "job_001",
                "00000001",  # extranonce2
                "0102030405060708"  # nonce (8 bytes hex)
            ]
        })

        resp = recv_json(sock)
        if not resp:
            print("[FAIL] No response to submit")
        elif resp.get('error'):
            # Expected - no valid job exists
            print(f"[OK] Submit rejected as expected: {resp['error']}")
        else:
            print(f"[OK] Submit response: {resp}")

        print("\n" + "="*60)
        print("Protocol Compatibility Test Complete")
        print("="*60 + "\n")

        return True

    finally:
        sock.close()

if __name__ == '__main__':
    host = sys.argv[1] if len(sys.argv) > 1 else '127.0.0.1'
    port = int(sys.argv[2]) if len(sys.argv) > 2 else 3333

    success = test_stratum_protocol(host, port)
    sys.exit(0 if success else 1)
