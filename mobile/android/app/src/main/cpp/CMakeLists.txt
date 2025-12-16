# android/app/src/main/cpp/CMakeLists.txt
cmake_minimum_required(VERSION 3.18.1)

project("translation-lib")

# 设置 C++ 标准
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# 添加翻译库
add_library(
    translation-lib
    SHARED
    translation-lib.cpp
    # TODO: 添加实际模型实现文件
    # whisper/whisper.cpp
    # opus/opus.cpp
    # tts/tts.cpp
)

# 查找依赖库
find_library(
    log-lib
    log
)

find_library(
    android-lib
    android
)

# 包含目录
target_include_directories(
    translation-lib
    PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}
    # TODO: 添加模型头文件目录
    # ${CMAKE_CURRENT_SOURCE_DIR}/whisper
    # ${CMAKE_CURRENT_SOURCE_DIR}/opus
    # ${CMAKE_CURRENT_SOURCE_DIR}/tts
)

# 链接库
target_link_libraries(
    translation-lib
    ${log-lib}
    ${android-lib}
)

# 编译选项
target_compile_options(
    translation-lib
    PRIVATE
    -Wall
    -Wextra
    -O3
    -ffast-math
)
