# Hosted preset (default): one D18 library per job.
# HTTP = libcurl. Crypto/hash/sign = OpenSSL. Zip = libzip. NFC = utf8proc.
# Do not find_package a second HTTP or JSON stack (no cpp-httplib, cpr, RapidJSON).

find_package(CURL REQUIRED)
find_package(OpenSSL REQUIRED)
find_package(PkgConfig REQUIRED)

pkg_check_modules(LIBZIP REQUIRED libzip)
pkg_check_modules(UTF8PROC REQUIRED libutf8proc)

target_link_libraries(kirivers_client
  PUBLIC
    CURL::libcurl
    OpenSSL::Crypto
    OpenSSL::SSL
  PRIVATE
    ${LIBZIP_LIBRARIES}
    ${UTF8PROC_LIBRARIES}
)

target_include_directories(kirivers_client
  PRIVATE
    ${LIBZIP_INCLUDE_DIRS}
    ${UTF8PROC_INCLUDE_DIRS}
)

target_compile_options(kirivers_client
  PRIVATE
    ${LIBZIP_CFLAGS_OTHER}
    ${UTF8PROC_CFLAGS_OTHER}
)

if(LIBZIP_LIBRARY_DIRS)
  target_link_directories(kirivers_client PRIVATE ${LIBZIP_LIBRARY_DIRS})
endif()
if(UTF8PROC_LIBRARY_DIRS)
  target_link_directories(kirivers_client PRIVATE ${UTF8PROC_LIBRARY_DIRS})
endif()
