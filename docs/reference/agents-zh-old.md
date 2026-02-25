# AGENTS.MD - AllCallAll 项目 AI 助手指南

## 项目概述

AllCallAll 是一个基于 WebRTC 的实时通话应用,采用前后端分离架构:
- **后端**: Go 1.22 + MySQL 8.0 + Redis 7.2 + Pion WebRTC
- **前端**: React Native + TypeScript + Expo
- **核心功能**: 实时音视频通话、用户认证(JWT)、在线状态管理、联系人管理

## 代码规范

### Go 后端规范

#### 1. 代码风格
- 严格遵循 Go 官方代码规范和 `gofmt` 格式化标准
- 使用 `golangci-lint` 进行代码检查
- 包名使用小写单数形式,如 `user`, `auth`, `signaling`
- 接口命名以 `-er` 结尾,如 `Repository`, `Service`

#### 2. 项目结构
```
backend/
├── cmd/                    # 应用入口
├── internal/              # 内部包(不可被外部导入)
│   ├── handlers/         # HTTP 处理器
│   ├── models/           # 数据模型
│   ├── auth/            # 认证相关
│   ├── database/        # 数据库连接
│   ├── cache/           # Redis 缓存
│   └── [domain]/        # 业务域(如 user, contact, signaling)
│       ├── repository.go # 数据访问层
│       └── service.go    # 业务逻辑层
└── configs/              # 配置文件
```

#### 3. 错误处理
- 使用标准库 `errors` 包和 `fmt.Errorf` 包装错误
- 在 handler 层统一处理错误响应
- 记录详细的错误日志,包含上下文信息
- 不要忽略任何错误,即使是 `defer` 中的错误

#### 4. 数据库操作
- 使用 Repository 模式封装数据访问
- 所有 SQL 查询使用参数化查询防止注入
- 长事务操作添加超时控制
- 合理使用索引,避免 N+1 查询

#### 5. Redis 缓存
- 所有缓存 key 使用统一前缀,如 `user:`, `session:`
- 设置合理的过期时间
- 使用 `SETEX` 或 `SET key value EX` 原子操作
- 在线状态使用 Redis 的 Hash 结构存储

### TypeScript/React Native 规范

#### 1. 代码风格
- 使用 TypeScript 严格模式
- 使用 ESLint + Prettier 保持代码一致性
- 组件使用函数式组件 + Hooks
- 文件名使用 PascalCase(组件)或 camelCase(工具函数)

#### 2. 项目结构
```
mobile/src/
├── api/              # API 客户端
├── components/       # 可复用组件
├── screens/          # 页面组件
├── context/          # React Context
├── navigation/       # 导航配置
└── config/          # 配置文件
```

#### 3. 状态管理
- 使用 React Context 管理全局状态(Auth, Signaling)
- 本地状态优先使用 `useState`, `useReducer`
- 异步操作使用 `useEffect` + 清理函数

#### 4. API 调用
- 所有 API 调用统一在 `src/api/` 目录
- 使用统一的 HTTP 客户端(`client.ts`)
- 处理请求/响应拦截器(token 注入、错误处理)
- 定义清晰的 TypeScript 类型

#### 5. WebRTC 最佳实践
- 信令连接使用 WebSocket,断线自动重连
- 正确处理 ICE 候选收集和交换
- 通话结束后及时释放媒体资源
- 处理权限请求(摄像头、麦克风)

## 开发工作流

### 1. 启动开发环境

**后端:**
```bash
cd backend
go run cmd/server/main.go
```

**前端:**
```bash
cd mobile
npm run start
# 另一个终端
./scripts/dev-client-debug.sh
```

**完整环境(推荐):**
```bash
./start.sh  # 启动后端 + 数据库 + Redis + 前端
```

### 2. 环境配置
- 后端配置: `backend/configs/config.yaml`
- 后端环境变量: `backend/.env` (从 `.env.example` 复制)
- 前端配置: `mobile/src/config/index.ts`
- Docker 配置: `infra/docker-compose.yml`

### 3. 数据库迁移
- 使用 SQL 文件手动迁移(暂无自动迁移工具)
- 在 `backend/internal/database/` 中维护表结构定义
- 重要字段变更需要在代码注释中说明

### 4. Git 工作流
- 功能分支命名: `feature/功能名称`
- 修复分支命名: `fix/问题描述`
- 提交信息格式: `类型: 简短描述`
  - `feat`: 新功能
  - `fix`: 修复 bug
  - `refactor`: 重构
  - `docs`: 文档更新
  - `test`: 测试相关

## 常见问题处理

### 1. 认证问题
- JWT token 存储在 React Native 的 AsyncStorage
- token 过期自动跳转登录页
- 后端中间件验证 token 并解析用户信息

### 2. WebRTC 连接问题
- 检查 STUN/TURN 服务器配置
- 确认信令服务 WebSocket 连接正常
- 查看浏览器/设备控制台的 ICE 状态

### 3. 在线状态同步
- 使用 Redis 存储在线状态
- WebSocket 心跳机制检测连接
- 定期清理过期的在线状态

### 4. 移动端调试
- Android: 使用 `adb reverse` 转发端口
- 使用 Expo Dev Client 进行原生调试
- 查看日志: `npx react-native log-android` 或 `log-ios`

## 测试规范

### 1. 后端测试
- 单元测试: 测试业务逻辑(Service 层)
- 集成测试: 测试 API 端点
- 使用 `testify` 断言库
- 测试文件命名: `*_test.go`

### 2. 前端测试
- 组件测试: 使用 React Native Testing Library
- E2E 测试: 使用 Detox(可选)
- 重要业务流程必须有测试覆盖

## 性能优化指南

### 1. 后端优化
- 使用 Redis 缓存热点数据
- 数据库查询优化:索引、批量操作
- 使用连接池管理数据库连接
- 避免在循环中进行 I/O 操作

### 2. 前端优化
- 使用 `React.memo` 避免不必要的重渲染
- 列表使用 `FlatList` 而非 `ScrollView`
- 图片优化:压缩、懒加载
- 减少不必要的状态更新

## 安全规范

### 1. 认证与授权
- 密码使用 bcrypt 加密存储
- JWT token 设置合理的过期时间
- 敏感操作需要二次验证
- API 接口实现权限控制

### 2. 数据验证
- 前后端都要进行数据验证
- 使用白名单而非黑名单
- 防止 SQL 注入、XSS 攻击
- 限制 API 请求频率

### 3. 通信安全
- 生产环境必须使用 HTTPS
- WebSocket 使用 WSS 加密
- 敏感数据传输加密

## 部署规范

### 1. 开发环境
- 使用 Docker Compose 本地部署
- 配置文件: `infra/docker-compose.yml`

### 2. 生产环境
- 使用 `infra/docker-compose.production.yml`
- 配置文件: `backend/configs/config.production.yaml`
- 使用环境变量管理敏感信息
- 配置 Nginx 反向代理
- 使用 Cloudflare Tunnel 暴露服务

### 3. 监控与日志
- 使用结构化日志
- 重要操作记录审计日志
- 监控服务健康状态
- 设置告警机制

## AI 助手特别注意事项

### 1. 代码修改
- 修改代码前先理解上下文
- 保持代码风格一致
- 修改后验证是否影响其他模块
- 重要修改需要添加注释说明

### 2. 依赖管理
- Go: 使用 `go mod tidy` 整理依赖
- Node.js: 使用 `npm install` 安装依赖
- 不要随意升级主要版本

### 3. 配置文件
- 不要提交敏感信息到 Git
- 使用 `.env.example` 作为模板
- 修改配置后通知相关开发者

### 4. 文档更新
- 代码变更同步更新相关文档
- 新功能添加使用说明
- 文档统一放在 `docs/` 目录

## 参考资源

- [Go 官方文档](https://go.dev/doc/)
- [React Native 文档](https://reactnative.dev/)
- [Pion WebRTC 文档](https://github.com/pion/webrtc)
- [Expo 文档](https://docs.expo.dev/)

---

**最后更新**: 2025-12-08
**维护者**: AllCallAll 开发团队
