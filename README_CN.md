# Agent Wrapper

> 一个运行在 TEE（可信执行环境）内的容器化服务，使任何 Agent 框架都能在 0G Citizen Claw 基础设施上运行，内置 TEE 安全、动态框架安装和响应签名功能。

## 它是什么

Agent Wrapper 是一个**预构建的容器镜像**，运行在 TEE 内部，负责管理 Agent 的生命周期。它处理所有复杂的工作：

- ✅ TEE 远程认证
- ✅ 安全密钥管理
- ✅ 加密配置获取
- ✅ **动态框架安装**
- ✅ 响应签名
- ✅ 心跳监控
- ✅ 健康检查

**您的 Agent 框架无需知道这些功能的存在。**

## 快速开始

### 1. 使用官方镜像

```bash
docker pull 0g-citizen-claw/agent-wrapper:latest
```

### 2. 部署到 0g-sandbox

```bash
sandbox start agent-wrapper:latest
```

### 3. 通过 HTTP 初始化

```bash
curl -X POST http://localhost:8080/_internal/init \
  -H "Content-Type: application/json" \
  -d '{
    "sealId": "0x1234...abcd",
    "tempKey": "0xabcd...",
    "attestorUrl": "https://attestor.example.com"
  }'
```

就这样，Wrapper 会自动：
- 执行远程认证
- 从区块链获取您的 Agent 元数据
- 动态安装所需的框架（openclaw、eliza 等）
- 启动您的 Agent
- 开始代理和签名响应

## 工作原理

```
┌─────────────────────────────────────────────────────────────┐
│                    0g-sandbox (TEE)                          │
│                                                              │
│  1. 启动官方镜像                                              │
│  2. HTTP 传入参数: {sealId, tempKey, attestorUrl}           │
│                         │                                    │
│                         ▼                                    │
│  ┌─────────────────────────────────────────────────────────┐│
│  │  Container                                              ││
│  │                                                          ││
│  │  ┌────────────────────────────────────────────────────┐││
│  │  │  Agent Wrapper (Go)                                │││
│  │  │  1. 通过 HTTP 接收初始化参数                        │││
│  │  │  2. 远程认证 → 获取密钥                             │││
│  │  │  3. 监听 sealId 绑定事件                            │││
│  │  │  4. 从区块链获取元数据                              │││
│  │  │  5. 从 Storage 获取加密配置                         │││
│  │  │  6. 动态安装框架 (pip/npm)                          │││
│  │  │  7. 启动 Agent 进程                                 │││
│  │  │  8. 代理 :8080 → :9000                              │││
│  │  │  9. 签名响应                                        │││
│  │  └────────────────────────────────────────────────────┘││
│  │                          │                              ││
│  │  ┌────────────────────────────────────────────────────┐││
│  │  │  Agent 框架 (动态安装)                              │││
│  │  │  - OpenClaw / Eliza / 自定义                        │││
│  │  │  - 无需感知 Wrapper                                 │││
│  │  │  - 只需处理 HTTP 请求                               │││
│  │  └────────────────────────────────────────────────────┘││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

## HTTP 初始化

Wrapper 通过简单的 HTTP 端点接收启动参数，而不是环境变量或命令行参数：

**POST** `/_internal/init`

```json
{
  "sealId": "0x1234...abcd",
  "tempKey": "0xabcd...",
  "attestorUrl": "https://attestor.example.com"
}
```

响应：
```json
{
  "status": "sealed",
  "message": "Entered sealed state, waiting for attestation"
}
```

## 链上元数据

您的 Agent 框架和版本信息注册在区块链上：

```json
{
  "framework": "openclaw",
  "version": "0.1.0",
  "configHash": "0x5678...",
  "imageHash": "0x9abc..."
}
```

Wrapper 读取此元数据并自动安装所需的框架。

## 您的 Agent 看到的内容

```python
# 您的 Agent 代码（无需修改）
from flask import Flask, request, jsonify

app = Flask(__name__)

@app.route("/chat", methods=["POST"])
def chat():
    data = request.json
    # 处理请求...
    return jsonify({"response": "hello!"})

if __name__ == "__main__":
    app.run(port=9000)  # Wrapper 转发到此端口
```

**无需导入 wrapper，无需签名代码，无需心跳逻辑。**

## 客户端收到的内容

```http
POST /chat HTTP/1.1
Host: your-agent.0g.io
Content-Type: application/json

{"message": "hello"}
```

```http
HTTP/1.1 200 OK
Content-Type: application/json
X-Agent-Id: 0x1234...
X-Signature: 0xabc... (ECDSA 签名)
X-Timestamp: 1712787654

{"response": "hello!"}
```

签名证明响应来自您的 TEE 保护的 Agent。

## 支持的框架

框架根据链上元数据动态安装：

| 框架 | 语言 | 安装方式 |
|------|------|----------|
| OpenClaw | Python | `pip install openclaw=={version}` |
| Eliza | Node.js | `npm install @eliza/core@{version}` |
| 自定义 | 任意 | 自定义安装脚本 |

## 架构

```
外部请求 → Wrapper (:8080) → Agent (:9000)
                           │
                           ├─ 远程认证
                           ├─ 获取加密配置
                           ├─ 管理私钥
                           ├─ 动态安装框架
                           ├─ 签名响应
                           └─ 发送心跳
```

## API 端点

### 内部（控制）

| 方法 | 路径 | 描述 |
|--------|------|-------------|
| POST | `/_internal/init` | 使用启动参数初始化 |
| GET | `/_internal/health` | 健康检查 |
| GET | `/_internal/ready` | 就绪检查 |

### 代理（流量）

| 方法 | 路径 | 描述 |
|--------|------|-------------|
| ANY | `/*` | 代理到 Agent 并签名响应 |

## 文档

- [用户指南](./docs/usage.md) - 部署和运行说明
- [架构图](./docs/architecture.md) - 可视化架构概览
- [设计文档](./docs/design.md) - 架构详情
- [API 参考](./docs/api.md) - HTTP 端点
- [开发指南](./docs/development.md) - 贡献指南
- [CLAUDE.md](./CLAUDE.md) - Claude Code 使用

## 验证模块

### 1. 本地构建和测试

```bash
# 进入项目目录
cd agent-wrapper

# 构建二进制文件
go build -o wrapper ./cmd/wrapper/

# 启动服务
./wrapper
```

### 2. 验证健康检查

```bash
# 新终端窗口
curl http://localhost:8080/_internal/health
```

预期响应：
```json
{
  "status": "healthy",
  "version": "0.2.0",
  "uptime": 1000
}
```

### 3. 验证初始化

```bash
curl -X POST http://localhost:8080/_internal/init \
  -H "Content-Type: application/json" \
  -d '{
    "sealId": "0x1234567890abcdef1234567890abcdef12345678",
    "tempKey": "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
    "attestorUrl": "https://attestor.example.com"
  }'
```

预期响应：
```json
{
  "status": "sealed",
  "message": "Entered sealed state, waiting for attestation"
}
```

### 4. 验证就绪状态

```bash
curl http://localhost:8080/_internal/ready
```

### 5. 运行测试套件

```bash
# 运行所有测试
go test ./...

# 运行测试并显示覆盖率
go test ./... -cover

# 运行测试并检测竞态条件
go test ./... -race

# 运行特定模块的测试
go test ./internal/blockchain/... -v
go test ./internal/config/... -v
go test ./internal/framework/... -v
go test ./internal/init/... -v
go test ./internal/sealed/... -v
```

### 6. 测试覆盖率报告

当前测试覆盖率（全部超过 80% 目标）：

| 模块 | 覆盖率 | 状态 |
|------|--------|------|
| blockchain | 84.8% | ✅ |
| config | 88.2% | ✅ |
| framework | 93.4% | ✅ |
| init | 85.8% | ✅ |
| sealed | 88.2% | ✅ |
| storage | 94.4% | ✅ |

### 7. 集成测试脚本

```bash
#!/bin/bash
# integration-test.sh

echo "=== Agent Wrapper 集成测试 ==="

# 启动服务
./wrapper &
WRAPPER_PID=$!

# 等待服务启动
sleep 2

echo "1. 测试健康检查..."
curl -s http://localhost:8080/_internal/health | jq .

echo "2. 测试就绪检查（初始化前）..."
curl -s http://localhost:8080/_internal/ready | jq .

echo "3. 测试初始化..."
curl -s -X POST http://localhost:8080/_internal/init \
  -H "Content-Type: application/json" \
  -d '{
    "sealId": "0x1234567890abcdef1234567890abcdef12345678",
    "tempKey": "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
    "attestorUrl": "https://attestor.example.com"
  }' | jq .

echo "4. 测试就绪检查（初始化后）..."
curl -s http://localhost:8080/_internal/ready | jq .

# 清理
kill $WRAPPER_PID
echo "=== 测试完成 ==="
```

### 8. Docker 验证

```bash
# 构建镜像
docker build -t agent-wrapper:test .

# 运行容器
docker run -d -p 8080:8080 --name wrapper-test agent-wrapper:test

# 验证端点
curl http://localhost:8080/_internal/health

# 初始化
curl -X POST http://localhost:8080/_internal/init \
  -H "Content-Type: application/json" \
  -d '{
    "sealId": "0x1234567890abcdef1234567890abcdef12345678",
    "tempKey": "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
    "attestorUrl": "https://attestor.example.com"
  }'

# 查看日志
docker logs -f wrapper-test

# 清理
docker stop wrapper-test
docker rm wrapper-test
```

## 许可证

MIT
