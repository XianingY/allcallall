# 全项目文档结构整理计划

## 目标
1.  **消除冗余**: 合并 `mobile/docs/` 和 `docs/mobile/`，确保单一真值来源。
2.  **清理结构**: 移除 `docs/` 根目录下的散乱文件，归类到子目录。
3.  **统一索引**: 更新主文档索引以反映变更。

---

## 现状分析

### 1. 移动端文档冗余
-   `docs/mobile/` (主文档库) 和 `mobile/docs/` (代码库内) 高度重复。
-   `docs/mobile/README.md` (202 lines) 比 `mobile/docs/README.md` (157 lines) 更新、更完整。
-   **决策**: 以 `docs/mobile/` 为主，删除 `mobile/docs/`，在 `mobile/` 下保留 `README.md` 指向 `docs/mobile/`。

### 2. 根目录散乱文件
-   `docs/restricted-network.md`: 属于部署配置 -> 应移至 `docs/deployment/`。
-   `docs/ARCHIVE_SUMMARY.md`: 属于维护记录 -> 应移至 `docs/maintenance/`。

---

## 执行步骤

### Phase 1: 移动端文档整合 (优先级: 高)
1.  **比对合并**: 确保 `mobile/docs/` 中没有 `docs/mobile/` 缺失的独特内容（初步检查确认 `docs/mobile` 更全，但需二次确认文件列表）。
2.  **删除冗余**: 删除 `mobile/docs/` 目录。
3.  **创建指引**: 在 `mobile/` 目录下更新或创建 `README.md`，添加指向 `docs/mobile/` 的链接。

### Phase 2: 根目录清理 (优先级: 中)
1.  **移动文件**:
    -   `mv docs/restricted-network.md docs/deployment/restricted-network-setup.md` (重命名以符合规范)
    -   `mv docs/ARCHIVE_SUMMARY.md docs/maintenance/archive-summary.md`
    -   `mv agents.md docs/reference/agents.md` (根目录仅保留 README)
2.  **更新引用**: 搜索并更新项目中引用了这些文件的链接。

### Phase 3: 索引更新 (优先级: 中)
1.  **更新 `docs/README.md`**: 反映上述文件移动。
2.  **更新 `docs/mobile/README.md`**: 确保内部链接正确。

---

## 验证清单
- [ ] `mobile/docs/` 不复存在。
- [ ] `docs/` 根目录下除了目录和 `README.md` 外无其他 Markdown 文件（配置/规则文件除外）。
- [ ] 所有死链已修复 (grep 检查)。

---

## 委托指南
```typescript
delegate_task(
  category="writing",
  prompt="执行文档整理计划：合并移动端文档，清理 docs 根目录..."
)
```
