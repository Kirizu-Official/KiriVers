cmake_minimum_required(VERSION 3.16)
project(kirivers_client VERSION 0.1.0 LANGUAGES CXX)

# Official KiriVers C++ client SDK (orphan branch sdk/cpp).
# JSON is always nlohmann/json (vendored). Hosted links libcurl + OpenSSL +
# libzip + utf8proc. Embedded (KIRIVERS_EMBEDDED=ON) keeps nlohmann/json and
# requires injected Transport / Hasher / FileStore / ArchiveUnpacker /
# SignatureVerifier. Patcher and Replacer are interfaces in both presets.

option(KIRIVERS_EMBEDDED
  "Keep nlohmann/json only; inject HTTP/hash/zip/NFC adapters"
  OFF)

if(CMAKE_SOURCE_DIR STREQUAL CMAKE_CURRENT_SOURCE_DIR)
  set(KIRIVERS_BUILD_TESTS_DEFAULT ON)
else()
  set(KIRIVERS_BUILD_TESTS_DEFAULT OFF)
endif()
option(KIRIVERS_BUILD_TESTS "Build contract, adapter, and integration tests"
  ${KIRIVERS_BUILD_TESTS_DEFAULT})

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

add_library(kirivers_client
  src/error.cpp
  src/path.cpp
  src/url.cpp
  src/parse.cpp
  src/delta.cpp
  src/client.cpp
  src/updater.cpp
)
add_library(kirivers::client ALIAS kirivers_client)

target_include_directories(kirivers_client
  PUBLIC
    $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
    $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/third_party>
    $<INSTALL_INTERFACE:include>
)

target_compile_definitions(kirivers_client
  PUBLIC
    KIRIVERS_CLIENT_VERSION="${PROJECT_VERSION}"
)

if(MSVC)
  target_compile_options(kirivers_client PUBLIC /utf-8)
endif()

if(KIRIVERS_EMBEDDED)
  target_compile_definitions(kirivers_client PUBLIC KIRIVERS_EMBEDDED=1)
else()
  target_compile_definitions(kirivers_client PUBLIC KIRIVERS_HOSTED=1)
  target_sources(kirivers_client PRIVATE
    src/hosted/curl_transport.cpp
    src/hosted/openssl_crypto.cpp
    src/hosted/filesystem_store.cpp
    src/hosted/libzip_unpacker.cpp
    src/hosted/defaults.cpp
  )
  if(WIN32)
    target_sources(kirivers_client PRIVATE src/hosted/windows_replacer.cpp)
  endif()
  include("${CMAKE_CURRENT_SOURCE_DIR}/cmake/hosted.cmake")
endif()

if(KIRIVERS_BUILD_TESTS)
  enable_testing()
  add_subdirectory(tests)
endif()

include(GNUInstallDirs)
install(TARGETS kirivers_client
  EXPORT kirivers_clientTargets
  ARCHIVE DESTINATION ${CMAKE_INSTALL_LIBDIR}
  LIBRARY DESTINATION ${CMAKE_INSTALL_LIBDIR}
  RUNTIME DESTINATION ${CMAKE_INSTALL_BINDIR}
)
install(DIRECTORY include/kirivers DESTINATION ${CMAKE_INSTALL_INCLUDEDIR})
install(DIRECTORY third_party/nlohmann DESTINATION ${CMAKE_INSTALL_INCLUDEDIR})
install(FILES openapi.client.json OPENAPI_REVISION README.md
  DESTINATION ${CMAKE_INSTALL_DATADIR}/kirivers-client
)
install(EXPORT kirivers_clientTargets
  FILE kirivers_clientTargets.cmake
  NAMESPACE kirivers::
  DESTINATION ${CMAKE_INSTALL_LIBDIR}/cmake/kirivers_client
)
