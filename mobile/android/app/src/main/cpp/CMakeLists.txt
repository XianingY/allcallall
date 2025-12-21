# android/app/src/main/cpp/CMakeLists.txt
cmake_minimum_required(VERSION 3.22.1)

project("translation-lib")

# 设置 C++ 标准
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# 计算 whisper.cpp 的绝对路径
# cpp -> main -> src -> app -> android -> mobile -> allcallall (6 级)
get_filename_component(PROJECT_ROOT "${CMAKE_CURRENT_SOURCE_DIR}/../../../../../.." ABSOLUTE)
set(WHISPER_CPP_DIR "${PROJECT_ROOT}/whisper.cpp")

message(STATUS "Looking for whisper.cpp at: ${WHISPER_CPP_DIR}")

# 检查 whisper.cpp 是否存在
if(EXISTS "${WHISPER_CPP_DIR}/src/whisper.cpp" AND EXISTS "${WHISPER_CPP_DIR}/ggml/src/ggml.c")
    message(STATUS "Found whisper.cpp - enabling full implementation")
    set(WHISPER_AVAILABLE ON)
else()
    message(WARNING "whisper.cpp not found - using placeholder implementation")
    set(WHISPER_AVAILABLE OFF)
endif()

# =============================================
# 如果 whisper.cpp 可用，编译完整版本
# =============================================
if(WHISPER_AVAILABLE)
    # GGML 核心源文件 (仅 CPU 后端，适合 Android)
    set(GGML_SOURCES
        "${WHISPER_CPP_DIR}/ggml/src/ggml.c"
        "${WHISPER_CPP_DIR}/ggml/src/ggml-alloc.c"
        "${WHISPER_CPP_DIR}/ggml/src/ggml-backend.cpp"
        "${WHISPER_CPP_DIR}/ggml/src/ggml-backend-reg.cpp"
        "${WHISPER_CPP_DIR}/ggml/src/ggml-opt.cpp"
        "${WHISPER_CPP_DIR}/ggml/src/ggml-quants.c"
        "${WHISPER_CPP_DIR}/ggml/src/ggml-threading.cpp"
        "${WHISPER_CPP_DIR}/ggml/src/gguf.cpp"
    )
    
    
    # GGML CPU 后端源文件 (包含所有必需的实现文件，包括架构特定实现)
    set(GGML_CPU_SOURCES
        "${WHISPER_CPP_DIR}/ggml/src/ggml-cpu/ggml-cpu.c"
        "${WHISPER_CPP_DIR}/ggml/src/ggml-cpu/ggml-cpu.cpp"
        "${WHISPER_CPP_DIR}/ggml/src/ggml-cpu/ops.cpp"
        "${WHISPER_CPP_DIR}/ggml/src/ggml-cpu/binary-ops.cpp"
        "${WHISPER_CPP_DIR}/ggml/src/ggml-cpu/unary-ops.cpp"
        "${WHISPER_CPP_DIR}/ggml/src/ggml-cpu/quants.c"
        "${WHISPER_CPP_DIR}/ggml/src/ggml-cpu/repack.cpp"
        "${WHISPER_CPP_DIR}/ggml/src/ggml-cpu/traits.cpp"
        "${WHISPER_CPP_DIR}/ggml/src/ggml-cpu/vec.cpp"
        "${WHISPER_CPP_DIR}/ggml/src/ggml-cpu/hbm.cpp"
        # Architecture‑specific implementations for ARM
        "${WHISPER_CPP_DIR}/ggml/src/ggml-cpu/arch/arm/quants.c"
        "${WHISPER_CPP_DIR}/ggml/src/ggml-cpu/arch/arm/repack.cpp"
        "${WHISPER_CPP_DIR}/ggml/src/ggml-cpu/arch/arm/cpu-feats.cpp"
    )
    
    # 添加 CPU 后端源文件
    if(EXISTS "${WHISPER_CPP_DIR}/ggml/src/ggml-cpu/ggml-cpu.c")
        list(APPEND GGML_SOURCES ${GGML_CPU_SOURCES})
        message(STATUS "Added ggml-cpu sources with all implementations")
    endif()
    
    # Whisper 源文件
    set(WHISPER_SOURCES
        "${WHISPER_CPP_DIR}/src/whisper.cpp"
    )
    
    # 添加翻译库 (包含 whisper 源码)
    add_library(
        translation-lib
        SHARED
        translation-lib.cpp
        whisper/whisper_service.cpp
        onnx/onnx_wrapper.cpp
        tts/piper_wrapper.cpp
        ${GGML_SOURCES}
        ${WHISPER_SOURCES}
    )
    
    
    # 定义宏启用完整 whisper 实现
    target_compile_definitions(translation-lib PRIVATE 
        WHISPER_AVAILABLE=1
        GGML_USE_CPU=1
        GGML_VERSION="1.0.0"
        GGML_COMMIT="embedded"
        WHISPER_VERSION="1.7.0"
    )
    
    # 添加 whisper 和 ggml 头文件目录
    target_include_directories(
        translation-lib
        PRIVATE
        ${CMAKE_CURRENT_SOURCE_DIR}
        ${CMAKE_CURRENT_SOURCE_DIR}/whisper
        ${CMAKE_CURRENT_SOURCE_DIR}/onnx
        ${CMAKE_CURRENT_SOURCE_DIR}/tts
        "${WHISPER_CPP_DIR}/include"
        "${WHISPER_CPP_DIR}/src"
        "${WHISPER_CPP_DIR}/ggml/include"
        "${WHISPER_CPP_DIR}/ggml/src"
        "${WHISPER_CPP_DIR}/ggml/src/ggml-cpu"
    )

else()
    # =============================================
    # 占位符实现 (whisper.cpp 不可用)
    # =============================================
    add_library(
        translation-lib
        SHARED
        translation-lib.cpp
        whisper/whisper_service.cpp
        onnx/onnx_wrapper.cpp
        tts/piper_wrapper.cpp
    )
    
    target_include_directories(
        translation-lib
        PRIVATE
        ${CMAKE_CURRENT_SOURCE_DIR}
        ${CMAKE_CURRENT_SOURCE_DIR}/whisper
        ${CMAKE_CURRENT_SOURCE_DIR}/onnx
        ${CMAKE_CURRENT_SOURCE_DIR}/tts
    )
endif()
# =============================================
# 通用配置
# =============================================

# ONNX Runtime 配置
set(ONNX_RUNTIME_DIR "${CMAKE_CURRENT_SOURCE_DIR}/libs/onnxruntime")

# 添加 ONNX Runtime 头文件目录
target_include_directories(
    translation-lib
    PRIVATE
    "${ONNX_RUNTIME_DIR}/headers"
)

# 导入 ONNX Runtime 预编译库
add_library(onnxruntime SHARED IMPORTED)
set_target_properties(onnxruntime PROPERTIES
    IMPORTED_LOCATION "${ONNX_RUNTIME_DIR}/jni/${ANDROID_ABI}/libonnxruntime.so"
)

# 查找依赖库
find_library(log-lib log)
find_library(android-lib android)

# 链接库
target_link_libraries(
    translation-lib
    ${log-lib}
    ${android-lib}
    onnxruntime
)

# 定义 ONNX Runtime 可用
target_compile_definitions(translation-lib PRIVATE ONNX_RUNTIME_AVAILABLE=1)

# 编译选项
target_compile_options(
    translation-lib
    PRIVATE
    -Wall
    -Wextra
    -Wno-unused-parameter
    -Wno-unused-variable
    -Wno-sign-compare
    -O3
    # -ffast-math # Removed to fix ggml compilation error
)

# 针对 ARM NEON 优化
if(${ANDROID_ABI} STREQUAL "arm64-v8a")
    target_compile_options(translation-lib PRIVATE -march=armv8-a+fp+simd)
    target_compile_definitions(translation-lib PRIVATE GGML_USE_NEON=1)
elseif(${ANDROID_ABI} STREQUAL "armeabi-v7a")
    target_compile_options(translation-lib PRIVATE -mfpu=neon -mfloat-abi=softfp)
    target_compile_definitions(translation-lib PRIVATE GGML_USE_NEON=1)
endif()
