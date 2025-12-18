#!/usr/bin/env python3
"""
Mock Stratum Server for Testing tosminer Protocol Compatibility

This server simulates tos-pool's stratum interface to test if tosminer
can properly connect, subscribe, authorize, and receive jobs.

Usage: python mock_stratum_server.py [port]
"""

import socket
import json
import sys
import threading
import time
import hashlib

class MockStratumServer:
    def __init__(self, port=3333):
        self.port = port
        self.server = None
        self.running = False
        self.session_id = 0
        self.clients = []
        self.current_job_id = 0
        self.difficulty = 1000000

    def start(self):
        """Start the mock server"""
        self.server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self.server.bind(('0.0.0.0', self.port))
        self.server.listen(5)
        self.running = True

        print(f"Mock Stratum Server listening on port {self.port}")
        print("Waiting for tosminer connections...")
        print("-" * 60)

        # Start job broadcaster
        job_thread = threading.Thread(target=self.job_broadcaster, daemon=True)
        job_thread.start()

        while self.running:
            try:
                client, addr = self.server.accept()
                print(f"\n[+] Connection from {addr}")
                self.session_id += 1
                handler = threading.Thread(
                    target=self.handle_client,
                    args=(client, addr, self.session_id),
                    daemon=True
                )
                self.clients.append(client)
                handler.start()
            except Exception as e:
                if self.running:
                    print(f"Accept error: {e}")

    def job_broadcaster(self):
        """Periodically broadcast new jobs"""
        while self.running:
            time.sleep(30)  # Broadcast new job every 30 seconds
            if self.clients:
                self.current_job_id += 1
                job = self.create_job()
                for client in self.clients[:]:
                    try:
                        self.send_notify(client, job)
                    except:
                        self.clients.remove(client)

    def create_job(self):
        """Create a mock job"""
        job_id = f"job_{self.current_job_id:06d}"

        # Create mock header (112 bytes = 224 hex chars)
        header = hashlib.sha256(f"header_{job_id}".encode()).hexdigest()
        header = header * 7  # Pad to 224 chars
        header = header[:224]

        # Create mock target (32 bytes = 64 hex chars)
        # Difficulty 1000000 means target = base / 1000000
        # base = 0x00000000FFFF << 208
        # Simple approximation for testing
        target = "00000000000000ffff" + "0" * 46

        return {
            "id": job_id,
            "header": header,
            "target": target,
            "height": 1000000 + self.current_job_id,
            "clean_jobs": True
        }

    def handle_client(self, client, addr, session_id):
        """Handle a single client connection"""
        extranonce1 = f"{session_id:08x}"
        extranonce2_size = 4
        authorized = False
        worker = "unknown"

        buffer = ""

        try:
            client.settimeout(300)  # 5 minute timeout

            while self.running:
                data = client.recv(4096)
                if not data:
                    break

                buffer += data.decode()

                while '\n' in buffer:
                    line, buffer = buffer.split('\n', 1)
                    if not line.strip():
                        continue

                    print(f"[{session_id}] <<< {line}")

                    try:
                        req = json.loads(line)
                        method = req.get('method', '')
                        req_id = req.get('id')
                        params = req.get('params', [])

                        if method == 'mining.subscribe':
                            # Handle subscribe
                            miner_sw = params[0] if params else "unknown"
                            print(f"[{session_id}] Miner software: {miner_sw}")

                            # Send subscribe result
                            result = [
                                [
                                    ["mining.notify", str(session_id)],
                                    ["mining.set_difficulty", str(session_id)]
                                ],
                                extranonce1,
                                extranonce2_size
                            ]
                            self.send_result(client, req_id, result)

                            # Send initial difficulty
                            self.send_json(client, {
                                "id": None,
                                "method": "mining.set_difficulty",
                                "params": [self.difficulty]
                            })

                        elif method == 'mining.authorize':
                            # Handle authorize
                            username = params[0] if params else ""
                            parts = username.split('.', 1)
                            address = parts[0]
                            worker = parts[1] if len(parts) > 1 else "default"

                            print(f"[{session_id}] Authorization: {address}.{worker}")

                            # Accept any address for testing
                            authorized = True
                            self.send_result(client, req_id, True)

                            # Send initial job
                            self.current_job_id += 1
                            job = self.create_job()
                            self.send_notify(client, job)

                        elif method == 'mining.submit':
                            # Handle share submission
                            print(f"[{session_id}] Share submitted: {len(params)} params")

                            if len(params) >= 4:
                                worker_name = params[0]
                                job_id = params[1]
                                extranonce2 = params[2]

                                if len(params) >= 5:
                                    # Standard format: [worker, job_id, en2, ntime, nonce]
                                    nonce = params[4]
                                    print(f"[{session_id}] Standard format: job={job_id}, nonce={nonce}")
                                else:
                                    # tosminer format: [worker, job_id, en2, nonce]
                                    nonce = params[3]
                                    print(f"[{session_id}] tosminer format: job={job_id}, nonce={nonce}")

                                # Accept share for testing
                                self.send_result(client, req_id, True)
                                print(f"[{session_id}] Share ACCEPTED!")
                            else:
                                self.send_error(client, req_id, -1, "Invalid params")

                        elif method == 'mining.ping':
                            # Keepalive
                            self.send_result(client, req_id, "pong")

                        elif method == 'mining.extranonce.subscribe':
                            self.send_result(client, req_id, True)

                        else:
                            print(f"[{session_id}] Unknown method: {method}")
                            self.send_error(client, req_id, -32601, "Method not found")

                    except json.JSONDecodeError as e:
                        print(f"[{session_id}] JSON parse error: {e}")

        except socket.timeout:
            print(f"[{session_id}] Timeout")
        except Exception as e:
            print(f"[{session_id}] Error: {e}")
        finally:
            print(f"[{session_id}] Disconnected")
            if client in self.clients:
                self.clients.remove(client)
            client.close()

    def send_json(self, client, obj):
        """Send JSON message"""
        msg = json.dumps(obj) + '\n'
        print(f"    >>> {msg.strip()}")
        client.sendall(msg.encode())

    def send_result(self, client, req_id, result):
        """Send result response"""
        self.send_json(client, {
            "id": req_id,
            "result": result,
            "error": None
        })

    def send_error(self, client, req_id, code, message):
        """Send error response"""
        self.send_json(client, {
            "id": req_id,
            "result": None,
            "error": [code, message, None]
        })

    def send_notify(self, client, job):
        """Send job notification (mining.notify)"""
        # Format: [job_id, header_hex, target, height, clean_jobs]
        self.send_json(client, {
            "id": None,
            "method": "mining.notify",
            "params": [
                job["id"],
                job["header"],
                job["target"],
                job["height"],
                job["clean_jobs"]
            ]
        })

    def stop(self):
        """Stop the server"""
        self.running = False
        if self.server:
            self.server.close()

if __name__ == '__main__':
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 3333

    server = MockStratumServer(port)
    try:
        server.start()
    except KeyboardInterrupt:
        print("\nShutting down...")
        server.stop()
