#!/bin/bash

# AllCallAll Alarm 功能验证脚本
# 此脚本用于验证alarm增强功能的所有组件是否正确配置

echo "=================================="
echo "AllCallAll Alarm 功能验证"
echo "=================================="
echo ""

# 检查必要文件
echo "📋 检查必要文件..."
files=(
    "src/services/VibrationService.ts"
    "src/services/PushNotificationService.ts"
    "src/services/AudioServiceExpo.ts"
    "src/context/SettingsContext.tsx"
    "src/screens/SettingsScreen.tsx"
    "ALARM_ENHANCEMENTS_SUMMARY.md"
    "AUDIO_FILES_SETUP.md"
)

all_exist=true
for file in "${files[@]}"; do
    if [ -f "$file" ]; then
        echo "  ✅ $file"
    else
        echo "  ❌ $file - 文件缺失"
        all_exist=false
    fi
done
echo ""

# 检查package.json依赖
echo "📦 检查package.json依赖..."
if grep -q "@react-native-firebase/messaging" package.json; then
    echo "  ✅ @react-native-firebase/messaging"
else
    echo "  ❌ @react-native-firebase/messaging - 依赖缺失"
    all_exist=false
fi

if grep -q "expo-av" package.json; then
    echo "  ✅ expo-av"
else
    echo "  ❌ expo-av - 依赖缺失"
    all_exist=false
fi
echo ""

# 检查音频文件目录
echo "🎵 检查音频文件目录..."
if [ -d "src/assets/sounds" ]; then
    echo "  ✅ src/assets/sounds/ 目录存在"
    audio_files=("incoming_call.wav" "outgoing_dial.wav" "ringback.wav")
    for audio in "${audio_files[@]}"; do
        if [ -f "src/assets/sounds/$audio" ]; then
            echo "    ✅ $audio"
        else
            echo "    ⚠️  $audio - 文件缺失 (可选)"
        fi
    done
else
    echo "  ❌ src/assets/sounds/ 目录不存在"
    all_exist=false
fi
echo ""

# 检查设置项
echo "⚙️ 检查设置项..."
if grep -q "vibrationEnabled" src/context/SettingsContext.tsx; then
    echo "  ✅ vibrationEnabled 设置项"
else
    echo "  ❌ vibrationEnabled 设置项缺失"
    all_exist=false
fi

if grep -q "pushNotificationsEnabled" src/context/SettingsContext.tsx; then
    echo "  ✅ pushNotificationsEnabled 设置项"
else
    echo "  ❌ pushNotificationsEnabled 设置项缺失"
    all_exist=false
fi
echo ""

# 检查服务类方法
echo "🔧 检查服务类方法..."
services=(
    "VibrationService:vibrate"
    "VibrationService:cancel"
    "PushNotificationService:requestPermission"
    "AudioServiceExpo:preloadAudioFiles"
    "AudioServiceExpo:play"
)

for service_method in "${services[@]}"; do
    service=$(echo $service_method | cut -d: -f1)
    method=$(echo $service_method | cut -d: -f2)
    if grep -q "public $method" "src/services/${service}.ts"; then
        echo "  ✅ $service.$method()"
    else
        echo "  ❌ $service.$method() - 方法缺失"
        all_exist=false
    fi
done
echo ""

# 总结
echo "=================================="
if [ "$all_exist" = true ]; then
    echo "✅ 所有核心组件验证通过！"
    echo ""
    echo "📝 后续步骤："
    echo "1. 添加音频文件到 src/assets/sounds/"
    echo "2. 配置Firebase项目和FCM"
    echo "3. 安装依赖: npm install"
    echo "4. 运行应用测试功能"
else
    echo "⚠️  部分组件缺失，请检查上述错误"
fi
echo "=================================="
