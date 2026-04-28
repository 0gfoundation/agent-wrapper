#!/usr/bin/env python3
"""
OpenClaw-compatible Agent Example

This is a simplified agent that demonstrates how the agent-wrapper
would work with an OpenClaw-based agent framework.

In production, this would be replaced by actual OpenClaw agent code
that uses the cmdop SDK and OpenClaw extensions.
"""
import os
import sys
import json
import logging
from http.server import HTTPServer, BaseHTTPRequestHandler
from typing import Optional, Dict, Any
import signal
import time

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='[%(asctime)s] [%(name)s] [%(levelname)s] %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'
)
logger = logging.getLogger("openclaw-agent")


class OpenClawAgent:
    """
    Simplified OpenClaw-compatible agent.

    In production, this would:
    1. Use cmdop.CMDOPClient for agent discovery
    2. Use openclaw extension for CMDOP-specific features
    3. Implement proper gRPC/HTTP endpoints
    """

    def __init__(self):
        self.agent_id = os.environ.get('AGENT_ID', 'unknown-agent')
        self.framework = os.environ.get('FRAMEWORK', 'openclaw')
        self.version = os.environ.get('AGENT_VERSION', '2026.3.20')
        self.port = int(os.environ.get('AGENT_PORT', 9000))

        # Configuration from environment
        self.config = self._load_config()

        # State
        self.running = False
        self.start_time: Optional[float] = None

        logger.info(f"Initializing {self.framework} agent v{self.version}")
        logger.info(f"Agent ID: {self.agent_id}")

    def _load_config(self) -> Dict[str, Any]:
        """Load agent configuration from environment variables."""
        config = {
            'model': os.environ.get('MODEL', 'gpt-4'),
            'temperature': float(os.environ.get('TEMPERATURE', '0.7')),
            'max_tokens': int(os.environ.get('MAX_TOKENS', '2048')),
            'api_key': os.environ.get('API_KEY', ''),
        }

        # Log config (without sensitive data)
        safe_config = config.copy()
        if safe_config['api_key']:
            safe_config['api_key'] = '***REDACTED***'
        logger.info(f"Configuration: {safe_config}")

        return config

    def start(self):
        """Start the agent server."""
        logger.info(f"Starting HTTP server on port {self.port}...")

        server = HTTPServer(('0.0.0.0', self.port), self._create_handler())
        self.running = True
        self.start_time = time.time()

        # Set up signal handlers
        signal.signal(signal.SIGTERM, lambda s, f: self._shutdown(server))
        signal.signal(signal.SIGINT, lambda s, f: self._shutdown(server))

        logger.info("Agent ready to accept requests")
        logger.info(f"Endpoints: http://localhost:{self.port}/")

        try:
            server.serve_forever()
        except KeyboardInterrupt:
            logger.info("Received interrupt signal")
        finally:
            self._shutdown(server)

    def _create_handler(self):
        """Create HTTP request handler with access to agent instance."""
        agent = self

        class AgentHandler(BaseHTTPRequestHandler):
            def log_message(self, format, *args):
                """Use custom logger instead of stderr."""
                logger.info(f"[HTTP] {format % args}")

            def do_GET(self):
                """Handle GET requests."""
                if self.path == '/dashboard' or self.path == '/':
                    # Serve dashboard
                    self._serve_dashboard()
                elif self.path == '/health':
                    self._send_json({
                        'status': 'healthy' if agent.running else 'starting',
                        'agent_id': agent.agent_id,
                        'framework': agent.framework,
                        'version': agent.version,
                        'uptime': time.time() - agent.start_time if agent.start_time else 0
                    })
                elif self.path == '/status':
                    self._send_json({
                        'status': 'running' if agent.running else 'stopped',
                        'uptime': time.time() - agent.start_time if agent.start_time else 0,
                        'config': agent.config
                    })
                else:
                    self._send_error(404, 'Not Found')

            def do_POST(self):
                """Handle POST requests."""
                if self.path == '/chat':
                    self._handle_chat()
                elif self.path == '/generate':
                    self._handle_generate()
                else:
                    self._send_error(404, 'Not Found')

            def _send_json(self, data):
                """Send JSON response."""
                self.send_response(200)
                self.send_header('Content-Type', 'application/json')
                self.end_headers()
                self.wfile.write(json.dumps(data).encode())

            def _send_error(self, code, message):
                """Send error response."""
                self.send_response(code)
                self.send_header('Content-Type', 'application/json')
                self.end_headers()
                self.wfile.write(json.dumps({'error': message}).encode())

            def _serve_dashboard(self):
                """Serve the dashboard HTML."""
                import os
                dashboard_path = os.path.join(os.path.dirname(__file__), 'dashboard.html')
                try:
                    with open(dashboard_path, 'r', encoding='utf-8') as f:
                        content = f.read()
                    self.send_response(200)
                    self.send_header('Content-Type', 'text/html; charset=utf-8')
                    self.send_header('Cache-Control', 'no-cache')
                    self.end_headers()
                    self.wfile.write(content.encode())
                except FileNotFoundError:
                    logger.warning(f"Dashboard file not found: {dashboard_path}")
                    self._send_error(404, 'Dashboard not found')
                except Exception as e:
                    logger.error(f"Error serving dashboard: {e}")
                    self._send_error(500, str(e))

            def _handle_chat(self):
                """Handle chat completion request."""
                try:
                    content_length = int(self.headers.get('Content-Length', 0))
                    post_data = self.rfile.read(content_length) if content_length else b''

                    if not post_data:
                        self._send_error(400, 'Missing request body')
                        return

                    request = json.loads(post_data)
                    message = request.get('message', '')
                    session_id = request.get('session_id', 'default')

                    logger.info(f"Chat request: session={session_id}, message={message[:50]}...")

                    # Simulate agent processing (inline for demo)
                    response = f"OpenClaw agent (ID: {agent.agent_id}) received: {message}"

                    logger.info(f"Sending response: {response[:50]}...")

                    self._send_json({
                        'response': response,
                        'agent_id': agent.agent_id,
                        'framework': agent.framework,
                        'session_id': session_id,
                        'model': agent.config['model']
                    })

                except json.JSONDecodeError:
                    self._send_error(400, 'Invalid JSON')
                except Exception as e:
                    logger.error(f"Chat error: {e}", exc_info=True)
                    self._send_error(500, str(e))

            def _handle_generate(self):
                """Handle text generation request."""
                try:
                    content_length = int(self.headers.get('Content-Length', 0))
                    post_data = self.rfile.read(content_length) if content_length else b''

                    request = json.loads(post_data) if post_data else {}
                    prompt = request.get('prompt', '')
                    max_tokens = request.get('max_tokens', agent.config['max_tokens'])

                    logger.info(f"Generate request: prompt={prompt[:50]}...")

                    # Simulate text generation
                    response = f"[OpenClaw Generated Response]\nPrompt: {prompt}\n\nThis is a simulated response. In production, this would use the actual OpenClaw/CMDOP framework to generate responses using the configured model."

                    self._send_json({
                        'text': response,
                        'tokens_used': len(response.split()),
                        'model': agent.config['model']
                    })

                except Exception as e:
                    logger.error(f"Generate error: {e}")
                    self._send_error(500, str(e))

        return AgentHandler

    def _process_message(self, message: str, session_id: str) -> str:
        """
        Process a message from the user.

        In production, this would:
        1. Use OpenClaw's agent orchestration
        2. Call CMDOP's agent discovery
        3. Route to appropriate sub-agents
        4. Handle multi-turn conversations
        """
        # Simple echo with context for demo
        return f"OpenClaw agent (ID: {self.agent_id}) received: {message}"

    def _shutdown(self, server):
        """Gracefully shutdown the server."""
        if not self.running:
            return

        logger.info("Shutting down...")
        self.running = False
        server.shutdown()
        logger.info("Agent stopped")


def main():
    """Main entry point."""
    # Check for OpenClaw installation
    try:
        import openclaw
        logger.info(f"OpenClaw v{openclaw.__version__} is installed")
    except ImportError:
        logger.warning("OpenClaw not installed, running in compatibility mode")

    # Create and start agent
    agent = OpenClawAgent()

    try:
        agent.start()
    except Exception as e:
        logger.error(f"Agent error: {e}")
        sys.exit(1)


if __name__ == '__main__':
    main()
