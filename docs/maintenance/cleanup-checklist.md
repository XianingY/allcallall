# 项目文件清理检查清单

## 📋 待删除文件确认清单

请根据以下清单，确认您是否同意删除这些文件。

---

## 第一组：编译输出和二进制文件（高优先级 - 建议删除）

### ❌ `backend/allcall-server` 
- **类型**: 二进制可执行文件
- **大小**: 16.2 MB
- **用途**: 已编译的后端服务器可执行文件
- **何时生成**: `go build ./cmd/server` 时自动生成
- **是否需要版本控制**: ❌ 不需要，应该在 .gitignore 中
- **删除风险**: 🟢 零风险（运行时自动重新生成）
- **建议**: ✅ **强烈建议删除**

**删除命令**:
```bash
rm -f /Users/byzantium/github/allcallall/backend/allcall-server
```

---

### ❌ `backend/mail-test`
- **类型**: 二进制可执行文件
- **大小**: 7.7 MB
- **用途**: 邮件测试用的已编译文件
- **是否需要版本控制**: ❌ 不需要
- **删除风险**: 🟢 零风险
- **建议**: ✅ **强烈建议删除**

**删除命令**:
```bash
rm -f /Users/byzantium/github/allcallall/backend/mail-test
```

---

### ❓ `backend/wq`
- **类型**: 未知类型的文件
- **大小**: 1.2 KB
- **用途**: 不清楚（可能是临时测试文件）
- **如何使用**: 未知
- **删除风险**: 🟡 中等（如果未知用途，建议先确认）
- **建议**: 
  - 如果您不记得这个文件的用途，建议删除
  - 如果这是某个工具的输出，确认后再删除

**删除命令**:
```bash
rm -f /Users/byzantium/github/allcallall/backend/wq
```

---

## 第二组：空文件（高优先级 - 建议删除）

### ❌ `scripts/test-verification-code.go`
- **类型**: 空 Go 文件
- **大小**: 0 bytes
- **内容**: 完全为空
- **用途**: 不清楚（可能是遗留的空模板）
- **删除风险**: 🟢 零风险
- **建议**: ✅ **强烈建议删除**

**删除命令**:
```bash
rm -f /Users/byzantium/github/allcallall/scripts/test-verification-code.go
```

---

### ❌ `scripts/test-email-verification.sh`
- **类型**: 空 Shell 脚本
- **大小**: 0 bytes
- **内容**: 完全为空
- **实际实现**: 另有 `test-change-password.sh` 可以参考
- **删除风险**: 🟢 零风险
- **建议**: ✅ **强烈建议删除**

**删除命令**:
```bash
rm -f /Users/byzantium/github/allcallall/scripts/test-email-verification.sh
```

---

### ❌ `scripts/query-verification-code.sh`
- **类型**: 空 Shell 脚本
- **大小**: 0 bytes
- **内容**: 完全为空
- **删除风险**: 🟢 零风险
- **建议**: ✅ **强烈建议删除**

**删除命令**:
```bash
rm -f /Users/byzantium/github/allcallall/scripts/query-verification-code.sh
```

---

## 第三组：过时或有问题的脚本（中等优先级 - 有条件删除）

### ⚠️ `restart-services.sh` (根目录)
- **类型**: Shell 脚本
- **大小**: 3.0 KB
- **问题**: 
  - 路径错误: `PROJECT_ROOT="/Users/byzantium/github/allcall"` 
  - 应该是: `/Users/byzantium/github/allcallall`
  - 硬编码 IP 地址: `192.168.31.217` (可能已过期)
- **建议**: 
  - ❌ **先不删除**
  - ✅ **应该修复后移动到 `scripts/development/restart-services.sh`**

**修复内容示例**:
```bash
# 修复路径
PROJECT_ROOT="/Users/byzantium/github/allcallall"  # 改正拼写

# 更新 IP 地址检查（使用本机 IP）
curl -s http://localhost:8080/api/v1/health  # 改为本机 localhost
```

---

## 第四组：可选删除的文档（低优先级）

### 📄 `docs/document-move-summary.md`
- **类型**: 历史性文档
- **内容**: 描述之前的文档迁移工作
- **重要性**: 较低（仅作为历史参考）
- **建议**: 
  - 可以保留作为历史记录
  - 或移动到 `docs/maintenance/` 目录
  - 或直接删除（根据您的偏好）

---

### 📄 `docs/check.md`
- **类型**: 文档片段
- **大小**: 2.0 KB
- **内容**: 检查清单，具体内容不详（需要您查看）
- **重要性**: 未知
- **建议**: 
  - 需要您先查看内容
  - 如果仍有用，可以合并到其他文档中
  - 否则可以删除

---

## 🎯 推荐删除操作顺序

### ✅ 第一步：删除二进制文件（释放 24 MB）

```bash
rm -f backend/allcall-server
rm -f backend/mail-test
rm -f backend/wq
```

**风险等级**: 🟢 零风险

---

### ✅ 第二步：删除空文件

```bash
rm -f scripts/test-verification-code.go
rm -f scripts/test-email-verification.sh
rm -f scripts/query-verification-code.sh
```

**风险等级**: 🟢 零风险

---

### ⚠️ 第三步：处理过时脚本（可选）

```bash
# 选项 A：先修复再移动（推荐）
# 编辑 restart-services.sh，修复路径
# 然后移动到 scripts/development/

# 选项 B：暂时保留，稍后处理
# 保留此文件，待后续修复
```

**风险等级**: 🟡 中等（需要验证修复）

---

## 📊 删除影响汇总

| 项目 | 数量 | 空间 | 风险 |
|------|------|------|------|
| 二进制文件 | 3 | 24.1 MB | 🟢 低 |
| 空文件 | 3 | 0 KB | 🟢 低 |
| 过时脚本 | 1 | 3.0 KB | 🟡 中 |
| 历史文档 | 2 | ~6 KB | 🟢 低 |
| **总计** | **9** | **~24 MB** | **可接受** |

---

## ✋ 重要提醒

### 🔒 安全性考虑

1. **备份**: 如果您不确定，建议先创建一个 Git 分支或本地备份
2. **验证**: 删除前确认这些文件确实不需要
3. **Git 历史**: 即使删除，Git 历史中仍会保留记录

### 📌 关键问题

**问题 1**: `backend/wq` 文件是用什么生成的？
- 如果您不清楚，可以先保留

**问题 2**: `docs/check.md` 的内容是什么？
- 需要您查看后才能决定是否删除

**问题 3**: `restart-services.sh` 还需要用吗？
- 如果需要，建议修复后移动
- 如果不需要，可以删除

---

## ✅ 最终确认检查表

请将您的决定填入下表，然后提供给我进行最终执行：

```markdown
## 您的确认

### 二进制文件删除确认
- [ ] ✅ 同意删除 `backend/allcall-server`
- [ ] ✅ 同意删除 `backend/mail-test`
- [ ] ❓ 关于 `backend/wq`:
  - [ ] 删除它
  - [ ] 保留它（原因: ________)

### 空文件删除确认
- [ ] ✅ 同意删除 `scripts/test-verification-code.go`
- [ ] ✅ 同意删除 `scripts/test-email-verification.sh`
- [ ] ✅ 同意删除 `scripts/query-verification-code.sh`

### 过时脚本处理确认
- [ ] 修复并移动 `restart-services.sh`
- [ ] 先保留 `restart-services.sh`，稍后处理
- [ ] 删除 `restart-services.sh`

### 历史文档处理确认
- [ ] 删除 `docs/document-move-summary.md`
- [ ] 保留 `docs/document-move-summary.md`（作为历史记录）

### 检查清单处理确认
- [ ] 删除 `docs/check.md`（已确认无用）
- [ ] 保留 `docs/check.md`（原因: ________)
```

---

## 🚀 下一步

1. **请您确认上述文件是否可以删除**
2. **填写最终确认检查表**
3. **我会根据您的确认执行删除和重新组织**

---

**生成日期**: 2025-12-16  
**状态**: 📋 待您确认
