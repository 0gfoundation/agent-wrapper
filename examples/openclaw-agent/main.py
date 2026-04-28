#!/usr/bin/env python3
"""
简单的 OpenClaw Agent 示例
监听 9000 端口，提供 /chat 端点
"""

from flask import Flask, request, jsonify
from openclaw import Agent

app = Flask(__name__)

# 创建 OpenClaw Agent
agent = Agent(
    name="test-agent",
    description="A simple test agent"
)

@app.route("/chat", methods=["POST"])
def chat():
    """聊天端点"""
    data = request.json or {}
    message = data.get("message", "")

    # 使用 OpenClaw 处理消息
    response = agent.process(message)

    return jsonify({
        "response": response,
        "agent": "test-agent"
    })

@app.route("/health", methods=["GET"])
def health():
    """健康检查"""
    return jsonify({"status": "healthy"})

if __name__ == "__main__":
    # OpenClaw Agent 启动在 9000 端口
    # Wrapper 会代理 :8080 → :9000
    app.run(host="0.0.0.0", port=9000)
