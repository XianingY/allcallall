# android/app/src/main/cpp/CMakeLists.txt
cmake_minimum_required(VERSION 3.22.1)

project("translation-lib")

# 设置 C++ 标准
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# 计算 whisper.cpp 的绝对路径 (使用 third_party 子模块)
# cpp -> main -> src -> app -> android -> mobile -> allcallall (6 级)
get_filename_component(PROJECT_ROOT "${CMAKE_CURRENT_SOURCE_DIR}/../../../../../.." ABSOLUTE)
set(WHISPER_CPP_DIR "${PROJECT_ROOT}/third_party/whisper.cpp")

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
# SentencePiece 配置
# =============================================
set(SENTENCEPIECE_DIR "${PROJECT_ROOT}/third_party/sentencepiece")

if(EXISTS "${SENTENCEPIECE_DIR}/src/sentencepiece_processor.cc")
    message(STATUS "Found SentencePiece at: ${SENTENCEPIECE_DIR}")
    
    # SentencePiece 源文件 (仅包含推理所需的源文件)
    set(SP_SOURCES
        "${SENTENCEPIECE_DIR}/src/sentencepiece_processor.cc"
        # "${SENTENCEPIECE_DIR}/src/sentencepiece_model.proto" # 使用内置 pb.cc
        # "${SENTENCEPIECE_DIR}/src/sentencepiece_trainer.cc" # 不需要训练
        # "${SENTENCEPIECE_DIR}/src/trainer_interface.cc"
        "${SENTENCEPIECE_DIR}/src/unigram_model.cc"
        # "${SENTENCEPIECE_DIR}/src/unigram_model_trainer.cc"
        "${SENTENCEPIECE_DIR}/src/bpe_model.cc"
        # "${SENTENCEPIECE_DIR}/src/bpe_model_trainer.cc"
        "${SENTENCEPIECE_DIR}/src/word_model.cc"
        # "${SENTENCEPIECE_DIR}/src/word_model_trainer.cc"
        "${SENTENCEPIECE_DIR}/src/char_model.cc"
        # "${SENTENCEPIECE_DIR}/src/char_model_trainer.cc"
        "${SENTENCEPIECE_DIR}/src/model_interface.cc"
        "${SENTENCEPIECE_DIR}/src/model_factory.cc"
        "${SENTENCEPIECE_DIR}/src/normalizer.cc"
        "${SENTENCEPIECE_DIR}/src/util.cc"
        "${SENTENCEPIECE_DIR}/src/filesystem.cc"
        "${SENTENCEPIECE_DIR}/src/error.cc"
        # 内置 Protobuf 实现
        "${SENTENCEPIECE_DIR}/src/builtin_pb/sentencepiece_model.pb.cc"
        "${SENTENCEPIECE_DIR}/src/builtin_pb/sentencepiece.pb.cc"
    )
    
    # Protobuf Lite 配置
    set(PROTOBUF_LITE_DIR "${SENTENCEPIECE_DIR}/third_party/protobuf-lite")
    
    set(PROTOBUF_LITE_SOURCES
        "${PROTOBUF_LITE_DIR}/arena.cc"
        "${PROTOBUF_LITE_DIR}/arenastring.cc"
        "${PROTOBUF_LITE_DIR}/bytestream.cc"
        "${PROTOBUF_LITE_DIR}/coded_stream.cc"
        "${PROTOBUF_LITE_DIR}/common.cc"
        "${PROTOBUF_LITE_DIR}/extension_set.cc"
        "${PROTOBUF_LITE_DIR}/generated_enum_util.cc"
        "${PROTOBUF_LITE_DIR}/generated_message_table_driven_lite.cc"
        "${PROTOBUF_LITE_DIR}/generated_message_util.cc"
        "${PROTOBUF_LITE_DIR}/implicit_weak_message.cc"
        "${PROTOBUF_LITE_DIR}/int128.cc"
        "${PROTOBUF_LITE_DIR}/message_lite.cc"
        "${PROTOBUF_LITE_DIR}/parse_context.cc"
        "${PROTOBUF_LITE_DIR}/repeated_field.cc"
        "${PROTOBUF_LITE_DIR}/status.cc"
        "${PROTOBUF_LITE_DIR}/statusor.cc"
        "${PROTOBUF_LITE_DIR}/stringpiece.cc"
        "${PROTOBUF_LITE_DIR}/stringprintf.cc"
        "${PROTOBUF_LITE_DIR}/structurally_valid.cc"
        "${PROTOBUF_LITE_DIR}/strutil.cc"
        "${PROTOBUF_LITE_DIR}/time.cc"
        "${PROTOBUF_LITE_DIR}/wire_format_lite.cc"
        "${PROTOBUF_LITE_DIR}/zero_copy_stream.cc"
        "${PROTOBUF_LITE_DIR}/zero_copy_stream_impl.cc"
        "${PROTOBUF_LITE_DIR}/zero_copy_stream_impl_lite.cc"
    )

    # Internal ABSL 配置 (仅需 flag.cc 和 log.cc)
    set(ABSL_DIR "${SENTENCEPIECE_DIR}/third_party/absl")
    set(ABSL_SOURCES
        "${ABSL_DIR}/flags/flag.cc"
        "${ABSL_DIR}/log/log.cc"
    )

    # 添加 SentencePiece 头文件目录
    target_include_directories(
        translation-lib
        PRIVATE
        "${SENTENCEPIECE_DIR}/src"
        "${SENTENCEPIECE_DIR}/src/builtin_pb"
        "${SENTENCEPIECE_DIR}"
        "${SENTENCEPIECE_DIR}/third_party"  # for absl headers
        "${PROTOBUF_LITE_DIR}"
        "${CMAKE_CURRENT_SOURCE_DIR}/include" # for config.h
    )
    
    # 定义宏
    target_compile_definitions(translation-lib PRIVATE 
        _GLIBCXX_USE_CXX11_ABI=1
        _USE_INTERNAL_PROTOBUF=1
        _USE_INTERNAL_ABSL=1
        HAVE_PTHREAD=1
    )
    
    #添加 SentencePiece, Protobuf Lite 和 ABSL 源文件到库
    target_sources(translation-lib PRIVATE 
        ${SP_SOURCES}
        ${PROTOBUF_LITE_SOURCES}
        ${ABSL_SOURCES}
    )
    
    # 定义 SentencePiece 可用
    target_compile_definitions(translation-lib PRIVATE SENTENCEPIECE_AVAILABLE=1)
    
else()
    message(WARNING "SentencePiece not found at: ${SENTENCEPIECE_DIR}")
endif()

# =============================================
# espeak-ng 配置 (用于 Piper TTS)
# =============================================
set(ESPEAK_NG_DIR "${PROJECT_ROOT}/third_party/espeak-ng")
if(EXISTS "${ESPEAK_NG_DIR}/src/libespeak-ng")
    message(STATUS "Found espeak-ng at: ${ESPEAK_NG_DIR}")
    
    # espeak-ng 核心源文件
    set(ESPEAK_SOURCES
        "${ESPEAK_NG_DIR}/src/libespeak-ng/common.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/mnemonics.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/error.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/ieee80.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/compiledict.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/compiledata.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/dictionary.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/encoding.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/intonation.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/langopts.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/numbers.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/phoneme.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/phonemelist.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/readclause.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/setlengths.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/soundicon.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/spect.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/ssml.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/synthdata.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/synthesize.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/tr_languages.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/translate.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/translateword.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/voices.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/wavegen.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/speech.c"
        "${ESPEAK_NG_DIR}/src/libespeak-ng/espeak_api.c"
        # ucd-tools
        "${ESPEAK_NG_DIR}/src/ucd-tools/src/case.c"
        "${ESPEAK_NG_DIR}/src/ucd-tools/src/categories.c"
        "${ESPEAK_NG_DIR}/src/ucd-tools/src/ctype.c"
        "${ESPEAK_NG_DIR}/src/ucd-tools/src/proplist.c"
        "${ESPEAK_NG_DIR}/src/ucd-tools/src/scripts.c"
        "${ESPEAK_NG_DIR}/src/ucd-tools/src/tostring.c"
    )
    
    # espeak-ng 头文件目录
    target_include_directories(
        translation-lib
        PRIVATE
        "${ESPEAK_NG_DIR}/src/include"
        "${ESPEAK_NG_DIR}/src/include/compat"
        "${ESPEAK_NG_DIR}/src/ucd-tools/src/include"
        "${CMAKE_CURRENT_SOURCE_DIR}/include"  # for espeak_config.h -> config.h
    )
    
    # espeak-ng 编译定义
    target_compile_definitions(translation-lib PRIVATE
        ESPEAK_NG_AVAILABLE=1
        LIBESPEAK_NG_EXPORT=1
        # 数据路径将在运行时设置
        PATH_ESPEAK_DATA="/data/data/com.allcallall/files/espeak-ng-data"
    )
    
    # 添加 espeak-ng 源文件
    target_sources(translation-lib PRIVATE ${ESPEAK_SOURCES})
    
    message(STATUS "espeak-ng configured for TTS phoneme conversion")
else()
    message(WARNING "espeak-ng not found at: ${ESPEAK_NG_DIR}")
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
