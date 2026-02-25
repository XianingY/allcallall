# 🛠️ 故障排除

### 依赖冲突

```bash
# 清除 node_modules 重新安装
rm -rf node_modules package-lock.json
npm install
```

### 构建失败

```bash
# 清除 Gradle 缓存
cd android && ./gradlew clean

# 重新构建
./gradlew assembleDebug
```

### 环境检测失败

```bash
# 运行环境验证脚本
./scripts/verify-app-env.sh
./scripts/verify-alarm-setup.sh
```
