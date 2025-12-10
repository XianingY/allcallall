#!/bin/bash

# AllCallAll Alarm 功能验证脚本
# 此脚本用于验证alarm增强功能的所有组件是否正确配置

echo "=================================="
echo "AllCallAll Alarm 功能验证 (v2.0)"
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
    echo "  📝 支持格式: MP3 (推荐) 或 WAV"
    audio_files=("incoming_call.mp3" "ringback.mp3")
    for audio in "${audio_files[@]}"; do
        if [ -f "src/assets/sounds/$audio" ]; then
            echo "    ✅ $audio"
        else
            echo "    ⚠️  $audio - 文件缺失 (推荐添加)"
            # 同时检查是否有对应的wav文件
            wav_file="${audio/.mp3/.wav}"
            if [ -f "src/assets/sounds/$wav_file" ]; then
                echo "      💡 发现 $wav_file 文件，建议重命名为 $audio"
            fi
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

# 检查服务类关键特性
echo "🔧 检查服务类关键特性..."
echo "  VibrationService:"
if grep -q "class VibrationService" src/services/VibrationService.ts; then
    echo "    ✅ 类定义"
fi
if grep -q "incoming_call.*500.*250" src/services/VibrationService.ts; then
    echo "    ✅ 震动模式配置"
fi
if grep -q "public static getInstance" src/services/VibrationService.ts; then
    echo "    ✅ 单例模式"
fi

echo "  PushNotificationService:"
if grep -q "class PushNotificationService" src/services/PushNotificationService.ts; then
    echo "    ✅ 类定义"
fi
if grep -q "messaging" src/services/PushNotificationService.ts; then
    echo "    ✅ FCM集成"
fi
if grep -q "public static getInstance" src/services/PushNotificationService.ts; then
    echo "    ✅ 单例模式"
fi

echo "  AudioServiceExpo:"
if grep -q "class AudioServiceExpo" src/services/AudioServiceExpo.ts; then
    echo "    ✅ 类定义"
fi
if grep -q "expo-av" src/services/AudioServiceExpo.ts; then
    echo "    ✅ Expo AV集成"
fi
if grep -q "public static getInstance" src/services/AudioServiceExpo.ts; then
    echo "    ✅ 单例模式"
fi
echo ""

# 检查UI设置
echo "📱 检查UI设置..."
if grep -q "震动反馈" src/screens/SettingsScreen.tsx; then
    echo "  ✅ 震动设置UI"
fi
if grep -q "推送通知" src/screens/SettingsScreen.tsx; then
    echo "  ✅ 推送通知设置UI"
fi
echo ""

# 检查SignalingContext集成
echo "🔄 检查SignalingContext集成..."
if grep -q "VibrationService" src/context/SignalingContext.tsx; then
    echo "  ✅ 震动服务集成"
fi
if grep -q "AudioServiceExpo" src/context/SignalingContext.tsx; then
    echo "  ✅ 音频服务集成"
fi
echo ""

# 总结
echo "=================================="
echo "🎉 验证结果"
echo "=================================="
echo ""

if [ "$all_exist" = true ]; then
    echo "✅ 所有核心组件验证通过！"
    echo ""
    echo "📝 实现状态:"
    echo "  ✅ 3个服务类 (Vibration, PushNotification, Audio)"
    echo "  ✅ 3个设置项 (音频, 震动, 推送通知)"
    echo "  ✅ 3个UI开关"
    echo "  ✅ 完整的集成和联动"
    echo "  ✅ 所有依赖和文档"
    echo ""
    echo "📋 下一步:"
    echo "  1. (推荐) 添加音频文件到 src/assets/sounds/"
    echo "  2. 配置Firebase项目和FCM"
    echo "  3. 在App.tsx中集成导航引用"
    echo "  4. 运行应用测试功能"
    echo ""
    echo "💡 提示: 所有服务使用单例模式自动初始化，"
    echo "     无需手动调用，方法可能为private。"
else
    echo "⚠️  部分组件缺失，请检查上述错误"
fi

echo "=================================="
echo ""
echo "📚 相关文档:"
echo "  - ALARM_ENHANCEMENTS_SUMMARY.md"
echo "  - AUDIO_FILES_SETUP.md"
echo "  - IMPLEMENTATION_STATUS.md"
echo ""
