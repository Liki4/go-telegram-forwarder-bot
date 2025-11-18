# Go Telegram Forwarder Bot

一个功能完整的 Telegram Bot 系统，用于在不同用户和群组之间转发消息，支持管理多个转发 Bot 并提供详细的数据统计功能。

## ✨ 功能特性

### 核心功能
- **双层架构**：ManagerBot 管理多个 ForwarderBot，ForwarderBot 执行实际的消息转发
- **双向转发**：Guest → Recipients（入向），Recipients → Guest（出向）
- **多级权限**：Superuser、Manager、Admin、Guest 四级权限体系
- **黑名单管理**：支持审批流程，自动过期处理
- **实时统计**：消息转发量、用户数等实时统计
- **错误处理**：自动重试、失败通知、关键错误告警

### 高级特性
- **动态 Bot 管理**：支持运行时动态启动/停止 ForwarderBot，无需重启应用
- **限流保护**：Telegram API 限流（25条/秒）和 Guest 消息限流（1条/秒）
- **重试机制**：网络错误、429、5xx 自动重试（最多10次，间隔30秒）
- **群组监控**：自动检测无效群组并清理
- **Token 加密**：Bot Token 使用 AES-256 加密存储
- **审计日志**：关键操作永久记录
- **Redis 支持**：可选 Redis 用于限流和缓存
- **Proxy 支持**：支持 HTTP/HTTPS/SOCKS5 代理，适用于无法直接访问 Telegram API 的网络环境
- **Markdown 安全**：自动转义用户输入中的 Markdown 特殊字符，防止格式错误
- **详细日志**：完整的 debug 级别日志，记录所有操作和状态变化

## 🏗️ 系统架构

### 架构图

```
┌─────────────────────────────────────────┐
│         ManagerBot (管理 Bot)           │
│  - 管理多个 ForwarderBot                │
│  - 用户权限管理                          │
│  - 全局统计                              │
└─────────────────────────────────────────┘
              │
              │ 管理
              ▼
┌─────────────────────────────────────────┐
│      ForwarderBot 1, 2, 3... (转发 Bot) │
│  - 执行实际的消息转发                    │
│  - 管理 Recipients                      │
│  - 黑名单管理                            │
│  - Bot 级别统计                          │
└─────────────────────────────────────────┘
```

### 角色体系

**ManagerBot 层：**
- **Superuser**：系统超级管理员（配置文件指定）
- **Manager**：ForwarderBot 的拥有者

**ForwarderBot 层：**
- **Manager**：Bot 拥有者
- **Admin**：管理员（由 Manager 添加）
- **Guest**：发送消息的普通用户
- **Recipient**：接收消息的目标（用户或群组）

## 📦 技术栈

- **Go 1.23+**
- **gotgbot/v2**：Telegram Bot API 客户端
- **GORM**：数据库 ORM（支持 SQLite、MySQL、PostgreSQL）
- **Viper**：配置管理
- **Zap**：结构化日志
- **go-redis**：Redis 客户端（可选）
- **UUID**：唯一标识符

## 🚀 快速开始

### 前置要求

- Go 1.23 或更高版本
- Telegram Bot Token（从 [@BotFather](https://t.me/BotFather) 获取）
- （可选）Redis 服务器

### 安装步骤

1. **克隆项目**
```bash
git clone https://github.com/Liki4/go-telegram-forwarder-bot
cd go-telegram-forwarder-bot
```

2. **安装依赖**
```bash
go mod tidy
```

3. **配置项目**
```bash
cp configs/config.yaml.example configs/config.yaml
```

4. **编辑配置文件**
编辑 `configs/config.yaml`，至少需要配置：
- `manager_bot.token`：ManagerBot 的 Token
- `manager_bot.superusers`：Superuser 的 Telegram User ID 列表
- `encryption_key`：加密密钥（见下方说明）

5. **生成加密密钥**（重要！）
```bash
openssl rand -base64 32
```
将生成的密钥填入配置文件的 `encryption_key` 字段。

**⚠️ 注意**：生产环境必须配置加密密钥，否则无法解密已存储的 Bot Token。

6. **构建项目**
```bash
go build ./cmd/bot
```

7. **运行**
```bash
./bot
```

## ⚙️ 配置说明

完整的配置示例请参考 `configs/config.yaml.example`。

### Proxy 配置

当网络无法直接访问 Telegram API 时，可以配置代理：

```yaml
proxy:
  enabled: true
  url: "http://127.0.0.1:7890"  # 代理地址
  username: ""                  # 可选：代理认证用户名
  password: ""                   # 可选：代理认证密码
```

**支持的代理类型：**
- HTTP/HTTPS 代理：`http://127.0.0.1:7890`
- SOCKS5 代理：`socks5://127.0.0.1:1080`
- 带认证的代理：`http://user:pass@proxy.example.com:8080`

**说明：**
- 启用代理后，所有 Telegram API 请求（包括 ManagerBot 和所有 ForwarderBot）都会通过代理
- 代理配置会在启动时验证，如果配置错误会立即报错
- 建议在生产环境使用稳定的代理服务

### 必需配置

```yaml
manager_bot:
  token: "YOUR_MANAGER_BOT_TOKEN"      # ManagerBot Token
  superusers: [123456789, 987654321]   # Superuser User ID 列表

encryption_key: "base64_encoded_32_byte_key"  # 加密密钥（必需）
```

### 可选配置

```yaml
database:
  type: "sqlite"          # sqlite, mysql, postgres
  dsn: "bot.db"           # 数据库连接字符串

redis:
  enabled: false          # 是否启用 Redis
  address: "localhost:6379"
  password: ""
  db: 0

rate_limit:
  telegram_api: 25        # Telegram API 限流（条/秒）
  guest_message: 1        # Guest 消息限流（条/秒）

retry:
  max_attempts: 10        # 最大重试次数
  interval_seconds: 30    # 重试间隔（秒）

log:
  level: "debug"          # debug, info, warn, error
  output: "stdout"        # stdout, file
  file_path: "bot.log"    # 日志文件路径

environment: "development"  # development, production

proxy:
  enabled: false          # 是否启用代理
  url: ""                # 代理地址，如 "http://127.0.0.1:7890" 或 "socks5://127.0.0.1:1080"
  username: ""           # 代理认证用户名（可选）
  password: ""            # 代理认证密码（可选）
```

## 📖 使用指南

### ManagerBot 命令

#### `/addbot <token>`
添加新的 ForwarderBot。

**示例：**
```
/addbot 123456789:ABCdefGHIjklMNOpqrsTUVwxyz
```

**说明：**
- 执行命令的用户将成为该 Bot 的 Manager
- Token 会通过 Telegram API 验证
- Token 会加密存储
- Bot 添加成功后会自动启动，无需重启应用

#### `/mybots`
列出当前 Manager 管理的所有 ForwarderBot。

**功能：**
- 显示 Bot 列表
- 点击 Bot 可查看详细信息
- 支持删除 Bot（需确认）

#### `/manage`（Superuser 专用）
打开管理界面。

**功能：**
- 查看所有 Manager（点击可查看 Manager 详情及其管理的所有 Bots）
- 查看所有 ForwarderBot（点击可查看 Bot 详细信息）
- 查看 Manager 详情（包括统计信息和 Bot 列表）
- 查看 Bot 详细信息（包括统计信息）
- 删除 Bot（需确认，删除后立即停止）
- 所有页面都有 Back 按钮，支持完整导航

#### `/stats`（Superuser 专用）
查看全局统计信息。

**统计内容：**
- Manager 数量
- ForwarderBot 数量
- 总转发消息量（入向/出向）
- 总 Guest 数量

#### `/help`
显示帮助信息，列出所有可用命令。

**说明：**
- 根据用户角色（Superuser/Manager/Guest）显示相应的命令列表
- ManagerBot 和 ForwarderBot 都有独立的帮助信息

### ForwarderBot 命令

#### `/addrecipient <chat_id>`
添加 Recipient（接收者）。

**示例：**
```
/addrecipient -1001234567890
```

**说明：**
- `chat_id` 可以是用户 ID 或群组 ID
- 群组 ID 通常为负数
- 不需要验证，直接添加

#### `/delrecipient <chat_id>`
删除 Recipient。

#### `/listrecipient`
列出所有 Recipient。

#### `/addadmin <user_id>`
添加 Admin。

**示例：**
```
/addadmin 123456789
```

**说明：**
- Admin 拥有除添加/删除 Admin 外的所有 Manager 权限

#### `/deladmin <user_id>`
删除 Admin。

#### `/listadmins`
列出所有 Admin。

#### `/stats`
查看该 Bot 的统计信息。

**统计内容：**
- 入向消息量（Guest → Recipients）
- 出向消息量（Recipients → Guest）
- Guest 数量

#### `/help`
显示帮助信息，列出所有可用命令。

**说明：**
- 根据用户角色（Manager/Admin/Guest）显示相应的命令列表

#### `/ban`（需 Reply）
将 Guest 加入黑名单。

**使用方式：**
1. Reply 一条 Guest 发送的消息
2. 发送 `/ban` 命令
3. Manager 会收到审批请求
4. Manager 点击 Approve/Reject 按钮

**说明：**
- 如果 Recipient 是用户，该用户可以使用 `/ban`
- 如果 Recipient 是群组，群组中所有用户都可以使用 `/ban`
- 审批超时 1 天后自动通过

#### `/unban`（需 Reply）
将 Guest 移出黑名单。

**使用方式：**
1. Reply 一条被 ban 的 Guest 的消息
2. 发送 `/unban` 命令
3. Manager 会收到审批请求

## 🔄 消息转发流程

### Guest 发送消息

```
Guest 发送消息
    ↓
ForwarderBot 接收
    ↓
检查黑名单 → 如果在黑名单，丢弃
    ↓
检查限流 → 如果超限，延迟发送
    ↓
并发转发给所有 Recipients
    ├─→ Recipient 1 (goroutine)
    ├─→ Recipient 2 (goroutine)
    └─→ Recipient 3 (goroutine)
    ↓
每个转发：
    ├─→ 检查 Telegram API 限流
    ├─→ 执行转发（带重试）
    ├─→ 存储消息映射
    └─→ 记录错误（如失败）
```

### Recipient 回复消息

```
Recipient 回复消息
    ↓
ForwarderBot 接收
    ↓
查找消息映射（通过 recipient_message_id）
    ↓
找到对应的 guest_chat_id 和 guest_message_id
    ↓
转发回复给 Guest
    ↓
存储回复映射
```

## 🧪 测试

### 运行测试

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./internal/utils/...
go test ./internal/service/message/...

# 运行测试并显示覆盖率
go test -cover ./...
```

### 测试覆盖

当前测试覆盖：
- ✅ 加密/解密功能
- ✅ 限流器（Telegram API 和 Guest 消息）
- ✅ 重试机制（各种错误场景）

## 📁 项目结构

```
go-telegram-forwarder-bot/
├── cmd/
│   └── bot/
│       └── main.go                 # 应用入口
├── internal/
│   ├── bot/                        # Bot 实例管理
│   │   ├── manager_bot.go          # ManagerBot 实现
│   │   ├── forwarder_bot.go        # ForwarderBot 实现
│   │   └── manager.go              # BotManager：动态管理 ForwarderBot 生命周期
│   ├── config/                     # 配置管理
│   │   ├── config.go               # 配置结构
│   │   └── loader.go               # 配置加载
│   ├── database/                   # 数据库层
│   │   ├── connection.go           # 数据库连接
│   │   ├── migration.go            # 数据库迁移
│   │   └── redis.go                # Redis 连接
│   ├── models/                     # 数据模型（8个）
│   │   ├── user.go
│   │   ├── forwarder_bot.go
│   │   ├── recipient.go
│   │   ├── guest.go
│   │   ├── blacklist.go
│   │   ├── bot_admin.go
│   │   ├── message_mapping.go
│   │   └── audit_log.go
│   ├── repository/                 # 数据访问层（8个）
│   │   └── *_repo.go
│   ├── service/                    # 业务逻辑层
│   │   ├── manager_bot/            # ManagerBot 服务
│   │   ├── forwarder_bot/          # ForwarderBot 服务
│   │   ├── message/                # 消息处理
│   │   │   ├── forwarder.go        # 消息转发
│   │   │   ├── rate_limiter.go     # 限流
│   │   │   └── retry.go            # 重试
│   │   ├── blacklist/              # 黑名单服务
│   │   ├── statistics/             # 统计服务
│   │   ├── error_notifier.go       # 错误通知
│   │   └── group_monitor.go        # 群组监控
│   ├── logger/                     # 日志封装
│   └── utils/                      # 工具函数
│       ├── encryption.go           # Token 加密
│       ├── proxy.go                # Proxy 工具
│       └── markdown.go             # Markdown 转义工具
├── configs/                        # 配置文件
│   └── config.yaml.example
└── go.mod                          # Go 模块定义
```

## 🔒 安全特性

1. **Token 加密**：Bot Token 使用 AES-256-GCM 加密存储
2. **权限控制**：多级权限体系，操作需授权
3. **审计日志**：关键操作永久记录
4. **错误通知**：关键错误自动通知 Superuser
5. **限流保护**：防止 API 滥用和消息轰炸
6. **Markdown 安全**：自动转义用户输入，防止 Markdown 注入和格式错误
7. **输入验证**：所有用户输入都经过验证和清理

## 🐛 故障排除

### 常见问题

#### 1. 无法解密 Token
**问题**：启动时提示无法解密 Token。

**解决方案**：
- 确保配置文件中 `encryption_key` 与创建 Bot 时使用的密钥一致
- 如果密钥丢失，需要重新添加所有 ForwarderBot

#### 2. Redis 连接失败
**问题**：Redis 连接失败。

**解决方案**：
- 检查 Redis 服务是否运行
- 检查配置中的 Redis 地址和密码
- 如果不需要 Redis，可以设置 `redis.enabled: false`

#### 3. 消息转发失败
**问题**：消息转发失败，收到失败通知。

**可能原因**：
- Bot 被 Recipient 阻止
- 群组已删除或 Bot 被移除
- 网络问题

**解决方案**：
- 检查 Bot 是否被阻止
- 检查群组状态
- 系统会自动检测并清理无效的 Recipient

#### 4. 限流问题
**问题**：消息发送被限流。

**解决方案**：
- 检查配置中的限流设置
- 对于 Guest 消息限流，系统会延迟发送
- Telegram API 限流会等待后重试

#### 5. Proxy 连接失败
**问题**：配置了代理但无法连接 Telegram API。

**可能原因**：
- 代理地址配置错误
- 代理服务未运行
- 代理认证信息错误
- 代理不支持 HTTPS 连接

**解决方案**：
- 检查代理服务是否正常运行
- 验证代理地址和端口是否正确
- 如果使用认证，检查用户名和密码
- 尝试使用 `curl` 测试代理连接：
  ```bash
  curl -x http://127.0.0.1:7890 https://api.telegram.org
  ```
- 查看日志中的 proxy 相关错误信息

## 📝 开发指南

### 代码规范

- 遵循 Go 官方代码规范
- 使用 `gofmt` 格式化代码
- 使用有意义的变量和函数名
- 添加必要的注释

### 添加新功能

1. 在相应的 `service` 包中添加业务逻辑
2. 在 `repository` 包中添加数据访问方法
3. 在 `models` 包中添加数据模型（如需要）
4. 更新配置结构（如需要）
5. 添加单元测试

### 数据库迁移

数据库迁移使用 GORM 的 `AutoMigrate`，在应用启动时自动执行。

**注意**：生产环境建议先备份数据库。

## 🚢 部署

### 单实例部署

项目设计为单实例部署，ManagerBot 和所有 ForwarderBot 运行在同一进程中。

### 动态 Bot 管理

系统支持运行时动态管理 ForwarderBot：
- **添加 Bot**：通过 `/addbot` 命令添加后，Bot 会立即启动，无需重启应用
- **删除 Bot**：通过管理界面删除 Bot 后，Bot 会立即停止并清理资源
- **自动恢复**：应用重启后会自动加载并启动所有已注册的 ForwarderBot

### 部署步骤

1. **构建二进制文件**
```bash
go build -o bot ./cmd/bot
```

2. **准备配置文件**
```bash
cp configs/config.yaml.example configs/config.yaml
# 编辑配置文件
```

3. **运行**
```bash
./bot
```

### 使用 systemd（Linux）

创建 `/etc/systemd/system/telegram-forwarder-bot.service`：

```ini
[Unit]
Description=Telegram Forwarder Bot
After=network.target

[Service]
Type=simple
User=your-user
WorkingDirectory=/path/to/go-telegram-forwarder-bot
ExecStart=/path/to/go-telegram-forwarder-bot/bot
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

启动服务：
```bash
sudo systemctl enable telegram-forwarder-bot
sudo systemctl start telegram-forwarder-bot
```

## 📊 监控和日志

### 日志级别

- **development**：DEBUG 级别，输出到控制台，记录所有操作细节
- **production**：INFO 级别，输出到文件

### 日志内容

系统提供详细的 debug 级别日志，包括：
- 所有消息接收、转发、发送操作
- 命令执行和参数
- Callback 处理和执行结果
- 错误和重试信息
- 数据库操作
- Bot 启动和停止事件

### 关键错误通知

以下错误会自动通知 Superuser：
- 数据库连接失败
- Bot Token 失效（401 错误）
- Redis 连接失败（如果启用）
- 系统级错误（panic）

通知防抖：同一错误类型 1 小时内最多通知一次。

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

MIT License

## 🙏 致谢

- [gotgbot](https://github.com/PaulSonOfLars/gotgbot) - Telegram Bot API 客户端
- [GORM](https://gorm.io/) - Go ORM
- [Viper](https://github.com/spf13/viper) - 配置管理
- [Zap](https://github.com/uber-go/zap) - 结构化日志

---

**注意**：本项目仅供学习和研究使用。使用 Telegram Bot API 时请遵守 Telegram 的服务条款。
