#pragma once
#include <array>
#include <cstdint>
#include <string>
namespace mihomotv {
using WgKey = std::array<uint8_t,32>;
std::string startSystemVpn(const WgKey& localPrivate,const WgKey& localPublic,const WgKey& peerPublic);
std::string stopSystemVpn();
}
