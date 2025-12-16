# 📱 移动端脚本使用指南

移动端验证和辅助脚本。

## 📋 脚本列表

### 🔍 `verify-app-env.sh`

**用途**: 验证应用环境配置是否正确

**执行方式**:
```bash
chmod +x ./verify-app-env.sh
./verify-app-env.sh
```

**检查项**:
- ✅ 环境变量配置
- ✅ 应用配置文件
- ✅ 依赖项状态
- ✅ 开发工具可用性

**输出示例**:
```
✅ 应用配置正确
✅ 环境变量已设置
⚠️  需要更新依赖：npm install
```

**常见问题**:
- 脚本权限不足 → 运行 `chmod +x verify-app-env.sh`
- 未找到配置文件 → 确保在 mobile/ 目录下运行
- 环境变量不正确 → 检查 `.env` 或 `app.json`

---

### 🔔 `verify-alarm-setup.sh`

**用途**: 验证来电铃声和音频文件是否正确配置

**执行方式**:
```bash
chmod +x ./verify-alarm-setup.sh
./verify-alarm-setup.sh
```

**检查项**:
- ✅ 音频文件完整性
- ✅ 文件格式和编码
- ✅ 文件路径配置
- ✅ 权限和可读性

**输出示例**:
```
✅ 铃声文件已找到: assets/sounds/incoming_call.mp3
✅ 音频格式正确: MP3, 128kbps
⚠️  缺少背景音乐: assets/sounds/background.mp3
```

**常见问题**:
- 音频文件不存在 → 确保文件在 `assets/sounds/` 目录
- 格式不支持 → 转换为 MP3 格式
- 权限不足 → 检查文件读权限

---

### 📱 其他脚本

#### `dev-client-debug.sh`
- **用途**: 完整启动脚本（推荐）
- **功能**: 自动处理 ADB 转发、缓存清理、Metro 启动

#### `pair-wireless.sh`
- **用途**: 无线配对调试
- **功能**: 配置无线 ADB 连接

#### `setup-wireless-debug.sh`
- **用途**: 无线调试设置
- **功能**: 设置无线调试环境

---

## 🚀 运行所有验证

```bash
# 一次性运行所有验证
chmod +x ./verify-*.sh
for script in verify-*.sh; do
  echo "=== 运行 $script ==="
  ./$script
  echo ""
done
```

---

## 📝 脚本开发指南

### 添加新脚本

1. 在 `scripts/` 目录创建脚本文件
2. 添加执行权限: `chmod +x script-name.sh`
3. 更新本文档的脚本列表
4. 确保脚本包含：
   - #!/bin/bash 头
   - 清晰的输出和错误提示
   - 适当的退出代码

### 脚本模板

```bash
#!/bin/bash

# 脚本描述
# 用途：xxx
# 用法：./script-name.sh [选项]

set -e  # 遇到错误时退出

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 函数定义
check_something() {
  echo -e "${GREEN}✅${NC} 检查项通过"
}

# 主逻辑
echo "开始验证..."
check_something
echo "验证完成"
```

---

## 🔗 相关链接

- [移动端文档](../docs/mobile/README.md)
- [音频配置](../docs/mobile/setup/AUDIO_FILES_SETUP.md)
- [环境变量](../docs/mobile/setup/APP_ENV_USAGE.md)

---

**最后更新**: 2025-12-16
