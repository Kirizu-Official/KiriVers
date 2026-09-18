cmake_minimum_required(VERSION 3.16)
project(kirivers-client VERSION 0.1.0 LANGUAGES C)

set(CMAKE_C_STANDARD 11)
set(CMAKE_C_STANDARD_REQUIRED ON)
set(CMAKE_C_EXTENSIONS OFF)

option(KIRIVERS_EMBEDDED "cJSON + function-pointer adapters only (no curl/openssl/libzip/utf8proc)" OFF)
option(KIRIVERS_BUILD_TESTS "Build unit/contract/integration tests" ON)

add_library(cjson STATIC vendor/cJSON/cJSON.c)
target_include_directories(cjson PUBLIC vendor/cJSON)
target_compile_definitions(cjson PUBLIC CJSON_HIDE_SYMBOLS)

set(KIRIVERS_CORE
    src/util.c
    src/path.c
    src/parse.c
    src/types_free.c
    src/http.c
    src/client.c
    src/capabilities.c
    src/updater.c
)

add_library(kirivers STATIC ${KIRIVERS_CORE})
target_include_directories(kirivers
    PUBLIC
        $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
        $<INSTALL_INTERFACE:include>
    PRIVATE
        ${CMAKE_CURRENT_SOURCE_DIR}/src
)
target_link_libraries(kirivers PUBLIC cjson)
target_compile_definitions(kirivers PRIVATE _CRT_SECURE_NO_WARNINGS)

if(KIRIVERS_EMBEDDED)
    target_compile_definitions(kirivers PUBLIC KIRIVERS_EMBEDDED=1)
else()
    find_package(CURL REQUIRED)
    find_package(OpenSSL REQUIRED)
    find_package(PkgConfig REQUIRED)
    pkg_check_modules(LIBZIP REQUIRED IMPORTED_TARGET libzip)
    pkg_check_modules(UTF8PROC IMPORTED_TARGET libutf8proc)
    if(NOT UTF8PROC_FOUND)
        pkg_check_modules(UTF8PROC REQUIRED IMPORTED_TARGET utf8proc)
    endif()

    target_sources(kirivers PRIVATE
        src/adapters/transport_curl.c
        src/adapters/filestore_stdio.c
        src/adapters/hasher_openssl.c
        src/adapters/unpacker_libzip.c
        src/adapters/verifier_openssl.c
        src/adapters/hosted.c
    )
    target_compile_definitions(kirivers PUBLIC KIRIVERS_HOSTED=1)
    target_link_libraries(kirivers PUBLIC CURL::libcurl OpenSSL::Crypto PkgConfig::LIBZIP PkgConfig::UTF8PROC)
    if(WIN32)
        target_link_libraries(kirivers PUBLIC ws2_32 crypt32)
    endif()
endif()

include(GNUInstallDirs)
install(TARGETS kirivers cjson EXPORT kiriversTargets
    ARCHIVE DESTINATION ${CMAKE_INSTALL_LIBDIR}
    LIBRARY DESTINATION ${CMAKE_INSTALL_LIBDIR}
)
install(DIRECTORY include/ DESTINATION ${CMAKE_INSTALL_INCLUDEDIR})
install(FILES openapi.client.json OPENAPI_REVISION DESTINATION ${CMAKE_INSTALL_DATADIR}/kirivers-client)

if(KIRIVERS_BUILD_TESTS)
    enable_testing()
    add_library(kirivers_mock OBJECT tests/mock_transport.c)
    target_include_directories(kirivers_mock PRIVATE include src vendor/cJSON tests)
    target_compile_definitions(kirivers_mock PRIVATE _CRT_SECURE_NO_WARNINGS)

    set(KIRIVERS_TESTS
        test_contract
        test_capabilities
        test_pack_poll
        test_path
        test_error
        test_delta
        test_updater
        test_integration
    )
    foreach(t ${KIRIVERS_TESTS})
        add_executable(${t} tests/${t}.c)
        target_include_directories(${t} PRIVATE include src vendor/cJSON tests)
        target_link_libraries(${t} PRIVATE kirivers kirivers_mock)
        target_compile_definitions(${t} PRIVATE
            _CRT_SECURE_NO_WARNINGS
            KIRIVERS_OPENAPI_PATH="${CMAKE_CURRENT_SOURCE_DIR}/openapi.client.json"
        )
        add_test(NAME ${t} COMMAND ${t} WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR})
    endforeach()
endif()
