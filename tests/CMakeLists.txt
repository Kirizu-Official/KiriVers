cmake_minimum_required(VERSION 3.16)

function(kirivers_add_test name src)
  add_executable(${name} ${src})
  target_link_libraries(${name} PRIVATE kirivers_client)
  target_include_directories(${name} PRIVATE ${PROJECT_SOURCE_DIR}/src)
  target_compile_definitions(${name} PRIVATE
    KIRIVERS_SOURCE_DIR="${PROJECT_SOURCE_DIR}"
  )
  add_test(NAME ${name} COMMAND ${name})
endfunction()

kirivers_add_test(kirivers_contract_test contract_test.cpp)
kirivers_add_test(kirivers_adapter_test adapter_test.cpp)

if(NOT KIRIVERS_EMBEDDED)
  kirivers_add_test(kirivers_integration_test integration_test.cpp)
endif()
