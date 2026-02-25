# PR 创建指南（Trunk-based）

当前仓库采用 Trunk-based 流程：

- `main` 是唯一长期主干分支
- 功能从 `feature/*` 分支开发
- 修复从 `hotfix/*` 分支开发
- 所有 PR 直接合并到 `main`

## 标准流程

1. 从 `main` 拉取最新代码并创建分支

```bash
git checkout main
git pull --ff-only origin main
git checkout -b feature/<short-topic>
```

2. 开发并提交

```bash
git add <files>
git commit -m "feat: <summary>"
```

3. 推送分支

```bash
git push -u origin feature/<short-topic>
```

4. 创建 PR（目标 `main`）

```bash
gh pr create --base main --head feature/<short-topic> --fill
```

## PR 描述建议

描述中至少包含以下 4 项：

1. 背景与目标（为什么改）
2. 主要改动（改了什么）
3. 验证结果（如何验证、命令输出）
4. 风险与回滚（可能影响与兜底方式）

可参考模板：`docs/pr/pr-description-template.md`

## 合并前检查

- 分支已 rebase/merge 到最新 `main`
- 本地验证通过（至少运行受影响模块的测试/类型检查）
- 不包含密钥、证书、私有配置
- 文档与脚本同步更新（如行为有变化）

## 合并后建议

- 如为发布节点，在 `main` 合并后创建版本 tag（例如 `v1.2.0`）
- 必要时创建 GitHub Release 并附变更说明
