#!/usr/bin/env python3
"""
Demo Agent for agent-wrapper testing.
A simple HTTP server that simulates an AI agent.
"""
import os
import json
import sys
from http.server import HTTPServer, BaseHTTPRequestHandler
from threading import Thread
import signal


class AgentHandler(BaseHTTPRequestHandler):
    """HTTP handler for demo agent."""

    def log_message(self, format, *args):
        """Suppress default logging."""
        pass

    def do_GET(self):
        """Handle GET requests."""
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            response = {
                'status': 'healthy',
                'agent_id': os.environ.get('AGENT_ID', 'unknown'),
                'framework': os.environ.get('FRAMEWORK', 'unknown'),
                'version': os.environ.get('AGENT_VERSION', '0.1.0')
            }
            self.wfile.write(json.dumps(response).encode())
        elif self.path == '/':
            self.send_response(200)
            self.send_header('Content-Type', 'text/html')
            self.end_headers()
            html = f"""
            <html>
            <head><title>Demo Agent</title></head>
            <body>
                <h1>Demo Agent</h1>
                <p>Agent ID: {os.environ.get('AGENT_ID', 'unknown')}</p>
                <p>Framework: {os.environ.get('FRAMEWORK', 'unknown')}</p>
                <p>Endpoints:</p>
                <ul>
                    <li><a href="/health">GET /health</a> - Health check</li>
                    <li><code>POST /chat</code> - Chat endpoint</li>
                </ul>
            </body>
            </html>
            """
            self.wfile.write(html.encode())
        else:
            self.send_response(404)
            self.end_headers()

    def do_POST(self):
        """Handle POST requests."""
        if self.path == '/chat':
            content_length = int(self.headers.get('Content-Length', 0))
            post_data = self.rfile.read(content_length) if content_length else b''

            try:
                data = json.loads(post_data) if post_data else {}
                message = data.get('message', '')
            except json.JSONDecodeError:
                message = ''

            response = {
                'response': f'Demo agent received: {message}',
                'agent_id': os.environ.get('AGENT_ID', 'unknown'),
                'framework': os.environ.get('FRAMEWORK', 'demo'),
                'timestamp': __import__('time').time()
            }

            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps(response).encode())
        else:
            self.send_response(404)
            self.end_headers()


def main():
    """Start the demo agent server."""
    port = int(os.environ.get('AGENT_PORT', 9000))
    agent_id = os.environ.get('AGENT_ID', 'demo-agent')
    framework = os.environ.get('FRAMEWORK', 'demo')

    print(f"[{agent_id}] Starting demo agent on port {port}...", flush=True)
    print(f"[{agent_id}] Framework: {framework}", flush=True)

    server = HTTPServer(('0.0.0.0', port), AgentHandler)

    # Handle shutdown gracefully
    def signal_handler(signum, frame):
        print(f"[{agent_id}] Received signal {signum}, shutting down...", flush=True)
        server.shutdown()
        sys.exit(0)

    signal.signal(signal.SIGTERM, signal_handler)
    signal.signal(signal.SIGINT, signal_handler)

    print(f"[{agent_id}] Ready to accept requests", flush=True)

    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print(f"[{agent_id}] Shutting down...", flush=True)
        server.shutdown()


if __name__ == '__main__':
    main()
