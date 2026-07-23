CXX ?= c++
CPPFLAGS += -Idecoder/simde -DOOZ_BUILD_DLL=1 -DPALWORLD_DECOMPRESS_ONLY=1
CXXFLAGS ?= -O3
CXXFLAGS += -std=c++17 -fno-exceptions -fno-rtti -fno-strict-aliasing

SOURCES := main.cpp decoder/kraken.cpp decoder/bitknit.cpp decoder/lzna.cpp
OUTPUT ?= palworld-save-decode

.PHONY: all clean

all: $(OUTPUT)

$(OUTPUT): $(SOURCES) decoder/stdafx.h decoder/log_lookup.h
	$(CXX) $(CPPFLAGS) $(CXXFLAGS) $(LDFLAGS) $(SOURCES) $(LDLIBS) -o $@

clean:
	rm -f -- $(OUTPUT)
