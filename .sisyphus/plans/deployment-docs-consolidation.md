# 部署文档与脚本整合工作计划

## 目标
1. 统一所有部署路径为 `/opt/allcallall`
2. 合并冗余的部署文档
3. 整合重复的部署脚本
4. 消除文档与脚本之间的不一致

---

## 背景分析

### 当前问题

#### 路径不一致
| 位置 | 发现的路径 | 应统一为 |
|------|-----------|---------|
| 部分文档 | `/opt/allcallallall` (笔误) | `/opt/allcallall` |
| `infra/deploy.sh` (旧) | `/opt/AllCallAll` | `/opt/allcallall` |
| 部分示例 | `/opt/allcall` | `/opt/allcallall` |

#### 文档冗余 (7个文件，80%内容重叠)
| 文档 | 用途 | 冗余度 |
|------|------|--------|
| `deployment-guide.md` | 主参考文档 | **低** (基础) |
| `production-setup-and-apk-build.md` | 本地/LAN部署 | **低** (基础) |
| `deployment-checklist.md` | 验证清单 | **高** (重复Guide) |
| `DEPLOYMENT_QUICK_REFERENCE.md` | 命令速查 | **高** (重复Guide) |
| `CLOUD_DEPLOYMENT_SUMMARY.md` | 架构概览 | **高** (重复Guide) |
| `APK_BUILD_QUICK_REFERENCE.md` | APK构建 | **中** (重复Local) |
| `DEPLOYMENT_SUCCESS_SUMMARY.md` | 历史记录 | **低** (存档) |

#### 脚本冗余 (3个脚本，功能重叠)
| 脚本 | 用途 | 问题 |
|------|------|------|
| `scripts/deployment/deploy-cloud.sh` | 远程服务器部署 | 包含占位符URL，未维护 |
| `scripts/deployment/init-cloud-deployment.sh` | 环境初始化 | 可合并到主脚本 |
| `infra/deploy.sh` | 一键部署(Cloudflare) | 与deploy-cloud.sh理念不同 |

---

## 工作阶段

### Phase 1: 路径统一 (优先级: 高)

#### Task 1.1: 搜索并修复所有错误路径
**目标**: 将所有 `/opt/allcallallall`、`/opt/AllCallAll`、`/opt/allcall` 替换为 `/opt/allcallall`

**涉及文件**:
```
docs/deployment/deployment-guide.md
docs/deployment/deployment-checklist.md
docs/deployment/DEPLOYMENT_QUICK_REFERENCE.md
docs/deployment/CLOUD_DEPLOYMENT_SUMMARY.md
docs/deployment/production-setup-and-apk-build.md
docs/configuration/configuration.md
docs/getting-started/unified-env-config.md
scripts/deployment/deploy-cloud.sh
scripts/deployment/init-cloud-deployment.sh
infra/deploy.sh
```

**验证**: `grep -r "/opt/allcall\|/opt/AllCallAll" --include="*.md" --include="*.sh" docs/ scripts/ infra/`

**成功标准**: Grep 返回空结果（除了 `/opt/allcallall`）

---

### Phase 2: 文档整合 (优先级: 高)

#### Task 2.1: 创建统一的云部署文档
**目标**: 合并4个云部署相关文档为1个

**合并策略**:
```
deployment-guide.md (保留，作为主文档)
  ├── 吸收 CLOUD_DEPLOYMENT_SUMMARY.md 的架构图
  ├── 吸收 DEPLOYMENT_QUICK_REFERENCE.md 的命令速查表
  └── 保持 deployment-checklist.md 为独立的检查清单（保留）
```

**操作**:
1. 将 `CLOUD_DEPLOYMENT_SUMMARY.md` 的架构图迁移到 `deployment-guide.md` 开头
2. 将 `DEPLOYMENT_QUICK_REFERENCE.md` 的命令速查合并为 `deployment-guide.md` 的附录
3. 删除 `CLOUD_DEPLOYMENT_SUMMARY.md`
4. 删除 `DEPLOYMENT_QUICK_REFERENCE.md`
5. 保留 `deployment-checklist.md`（运维时有独立价值）

**验证**: 确保合并后的文档包含所有关键信息

#### Task 2.2: 整合本地部署文档
**目标**: 合并APK构建相关文档

**操作**:
1. 将 `APK_BUILD_QUICK_REFERENCE.md` 内容合并到 `production-setup-and-apk-build.md`
2. 删除 `APK_BUILD_QUICK_REFERENCE.md`

#### Task 2.3: 归档历史文档
**操作**:
1. 将 `DEPLOYMENT_SUCCESS_SUMMARY.md` 移动到 `docs/archive/`（或删除）

#### Task 2.4: 更新文档索引
**操作**:
1. 更新 `docs/README.md` 中的文档链接
2. 更新 `docs/deployment/` 目录下任何交叉引用

---

### Phase 3: 脚本整合 (优先级: 中)

#### Task 3.1: 评估脚本保留策略
**决策矩阵**:

| 脚本 | 保留? | 理由 |
|------|-------|------|
| `deploy-cloud.sh` | ✅ 保留并更新 | 主要的自动化部署入口 |
| `init-cloud-deployment.sh` | ❌ 删除 | 功能已被 deploy-cloud.sh 覆盖 |
| `infra/deploy.sh` | ⚠️ 重命名 | 专用于 Cloudflare Tunnel 场景 |

#### Task 3.2: 更新 deploy-cloud.sh
**操作**:
1. 移除占位符 URL `https://github.com/yourusername/allcall.git`
2. 改为从环境变量或参数读取 Git 仓库地址
3. 确保默认路径为 `/opt/allcallall`

#### Task 3.3: 删除冗余脚本
**操作**:
1. 删除 `scripts/deployment/init-cloud-deployment.sh`
2. 更新 `scripts/README.md` 移除对该脚本的引用

#### Task 3.4: 重命名并更新 infra/deploy.sh
**操作**:
1. 重命名为 `deploy-cloudflare-tunnel.sh` 以明确用途
2. 更新脚本头部注释说明其专门用于 Cloudflare Tunnel 部署方式
3. 确保默认路径为 `/opt/allcallall`

---

### Phase 4: 配置标准化 (优先级: 中)

#### Task 4.1: 统一环境文件命名
**当前不一致**:
- `deploy-cloud.sh`: 使用 `.env`
- `infra/deploy.sh`: 使用 `infra/.env.production`

**决策**: 保持两种命名，但在文档中明确说明：
- `.env` 用于项目根目录（通用配置）
- `infra/.env.production` 用于 Docker Compose 专用配置

#### Task 4.2: 更新移动端配置路径引用
**当前不一致**:
- `deployment-guide.md`: 引用 `mobile/src/config/index.ts`
- `deployment-checklist.md`: 引用 `mobile/src/config/cloud.config.ts`

**操作**: 统一引用为 `mobile/src/config/index.ts`，因为这是实际的主配置入口

---

## 最终文件结构

### 整合后的文档结构
```
docs/deployment/
├── deployment-guide.md          # 主部署指南（包含架构图、命令速查）
├── deployment-checklist.md      # 部署验证清单（独立保留）
├── production-setup-and-apk-build.md  # 本地部署与APK构建
└── (已删除: CLOUD_DEPLOYMENT_SUMMARY.md)
└── (已删除: DEPLOYMENT_QUICK_REFERENCE.md)
└── (已删除: APK_BUILD_QUICK_REFERENCE.md)
└── (已移动: DEPLOYMENT_SUCCESS_SUMMARY.md → archive/)
```

### 整合后的脚本结构
```
scripts/deployment/
├── deploy-cloud.sh              # 主部署脚本（Nginx/UFW）
└── (已删除: init-cloud-deployment.sh)

infra/
├── deploy-cloudflare-tunnel.sh  # Cloudflare Tunnel 专用部署（重命名自 deploy.sh）
├── docker-compose.yml
├── docker-compose.production.yml
└── ...
```

---

## 验证清单

- [ ] `grep -r "/opt/allcall[^a]" docs/ scripts/ infra/` 返回空
- [ ] `grep -r "/opt/AllCallAll" docs/ scripts/ infra/` 返回空
- [ ] `grep -r "/opt/allcallallall" docs/ scripts/ infra/` 返回空
- [ ] 所有脚本通过 `bash -n` 语法检查
- [ ] `docs/README.md` 链接全部有效
- [ ] 删除的文档没有被其他文件引用

---

## 执行顺序

1. **Phase 1**: 路径统一（先修复所有路径问题）
2. **Phase 2**: 文档整合（合并冗余文档）
3. **Phase 3**: 脚本整合（清理冗余脚本）
4. **Phase 4**: 配置标准化（统一配置引用）
5. **验证**: 运行验证清单

---

## 风险与注意事项

1. **备份**: 在删除任何文件前确保 Git 提交干净
2. **交叉引用**: 合并文档前检查是否有外部链接指向被删除的文档
3. **用户影响**: 更新 README 和索引页面，引导用户到新文档位置
4. **脚本测试**: 修改脚本后应在测试环境验证

---

## 委托指南

### 推荐使用的 Agent 配置

```
delegate_task(
  category="writing",
  load_skills=[],
  prompt="合并部署文档..."
)
```

### 技能要求
- 文档写作和整理 → `category="writing"`
- 无需特殊技能加载（纯文档/脚本整理任务）
