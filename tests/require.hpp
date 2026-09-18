#pragma once

#include <iostream>
#include <cstdlib>

#define REQUIRE(cond)                                                          \
  do {                                                                         \
    if (!(cond)) {                                                             \
      std::cerr << "REQUIRE failed " << __FILE__ << ":" << __LINE__ << " "     \
                << #cond << std::endl;                                         \
      std::exit(1);                                                            \
    }                                                                          \
  } while (0)
