// Palworld Save Decode: isolated stdin/stdout wrapper for the ooz decoder.
// Copyright (C) 2026 Luke Holland
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.

#include <cerrno>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <limits>
#include <string_view>
#include <vector>

int Kraken_Decompress(const std::uint8_t *src, std::size_t src_len,
                      std::uint8_t *dst, std::size_t dst_len);

namespace {
constexpr std::size_t kSafeSpace = 64;
constexpr std::uint64_t kHardMaxBytes =
    static_cast<std::uint64_t>(std::numeric_limits<int>::max());

int fail(const char *message) {
  std::fprintf(stderr, "palworld-save-decode: %s\n", message);
  return 1;
}

bool parseSize(const char *value, std::size_t *size) {
  if (value == nullptr || *value == '\0' || *value == '-') {
    return false;
  }
  errno = 0;
  char *end = nullptr;
  const unsigned long long parsed = std::strtoull(value, &end, 10);
  if (errno != 0 || end == value || *end != '\0' || parsed == 0 ||
      parsed > kHardMaxBytes ||
      parsed > std::numeric_limits<std::size_t>::max() - kSafeSpace) {
    return false;
  }
  *size = static_cast<std::size_t>(parsed);
  return true;
}
} // namespace

int main(int argc, char **argv) {
  if (argc != 3 || std::string_view(argv[1]) != "--raw-size") {
    return fail("usage: palworld-save-decode --raw-size BYTES");
  }
  std::size_t rawSize = 0;
  if (!parseSize(argv[2], &rawSize)) {
    return fail("raw size must be within 1..2147483647 bytes");
  }

  std::vector<std::uint8_t> input;
  std::uint8_t chunk[64 * 1024];
  while (true) {
    const std::size_t count = std::fread(chunk, 1, sizeof(chunk), stdin);
    if (count > 0) {
      if (input.size() > kHardMaxBytes - count) {
        return fail("compressed input exceeds 2147483647 bytes");
      }
      input.insert(input.end(), chunk, chunk + count);
    }
    if (count != sizeof(chunk)) {
      if (std::ferror(stdin)) {
        return fail("could not read compressed input");
      }
      break;
    }
  }
  if (input.empty()) {
    return fail("compressed input is empty");
  }

  std::vector<std::uint8_t> output(rawSize + kSafeSpace);
  const int decoded =
      Kraken_Decompress(input.data(), input.size(), output.data(), rawSize);
  if (decoded < 0 || static_cast<std::size_t>(decoded) != rawSize) {
    return fail("decompression failed");
  }
  if (std::fwrite(output.data(), 1, rawSize, stdout) != rawSize ||
      std::fflush(stdout) != 0) {
    return fail("could not write decompressed output");
  }
  return 0;
}
