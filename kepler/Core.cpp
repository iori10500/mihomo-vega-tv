#include "Core.h"
#include "Vpn.h"

#include <arpa/inet.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <cerrno>
#include <cstring>
#include <fcntl.h>
#include <ifaddrs.h>
#include <net/if.h>
#include <dlfcn.h>
#include <fstream>
#include <iterator>
#include <mutex>
#include <string>
#include <sys/stat.h>
#include <unistd.h>


namespace {
const std::string directory = "/data/iori-mihomo";
const std::string config = directory + "/config.json";
const std::string serviceState = directory + "/service-state";
const std::string ownerFile = directory + "/vpn-peer.pub";
const std::string candidate = directory + "/pending.json";
std::mutex mutex;
bool active = false;
using Start = int (*)(const char*);
std::string bound;
using Stop = void (*)();
void* library = nullptr;
Start start = nullptr;
Stop stop = nullptr;
using StartLocal = int (*)(uint8_t*,uint8_t*,uint8_t*);
StartLocal startLocal = nullptr;
Stop stopLocal = nullptr;

std::string readFile(const std::string& path) {
    std::ifstream in(path);
    return {std::istreambuf_iterator<char>(in), std::istreambuf_iterator<char>()};
}

std::string storeFile(const std::string& path, const std::string& value) {
    if (mkdir(directory.c_str(), 0700) && errno != EEXIST) return strerror(errno);
    const std::string tmp = path + ".new";
    const int fd = open(tmp.c_str(), O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC, 0600);
    if (fd < 0) return strerror(errno);
    size_t offset = 0;
    int error = 0;
    while (offset < value.size()) {
        const auto wrote = write(fd, value.data() + offset, value.size() - offset);
        if (wrote < 0 && errno == EINTR) continue;
        if (wrote <= 0) { error = errno ? errno : EIO; break; }
        offset += static_cast<size_t>(wrote);
    }
    if (!error && fsync(fd)) error = errno;
    if (close(fd) && !error) error = errno;
    if (!error && rename(tmp.c_str(), path.c_str())) error = errno;
    if (error) { unlink(tmp.c_str()); return strerror(error); }
    return "";
}

}

namespace mihomotv {
Core::Core() = default;
Core::~Core() noexcept = default;

std::string Core::proxyHost() {
    ifaddrs* interfaces = nullptr;
    if (getifaddrs(&interfaces)) return "";
    std::string result;
    for (auto* i = interfaces; i; i = i->ifa_next) {
        if (!i->ifa_addr || i->ifa_addr->sa_family != AF_INET || !(i->ifa_flags & IFF_UP) ||
            (i->ifa_flags & IFF_LOOPBACK)) continue;
        if (strncmp(i->ifa_name, "wlan", 4) && strncmp(i->ifa_name, "eth", 3)) continue;
        char ip[INET_ADDRSTRLEN]{};
        auto* address = reinterpret_cast<sockaddr_in*>(i->ifa_addr);
        if (inet_ntop(AF_INET, &address->sin_addr, ip, sizeof(ip))) {result = ip; break;}
    }
    freeifaddrs(interfaces);
    return result;
}
std::string Core::setRequested(bool value) {
    const auto previous = readFile(serviceState);
    if (value && previous != "connected" && previous != "disconnected") {
        const auto error = storeFile(serviceState, "disconnected");
        if (!error.empty()) return error;
    }
    return storeFile(directory + "/enabled", value ? "1" : "0");
}
bool Core::requested() {return readFile(directory + "/enabled") == "1";}

std::string Core::routingMode() {
    const auto mode = readFile(directory + "/routing-mode");
    return mode.empty() ? "smart" : mode;
}
std::string Core::setRoutingMode(std::string mode) {
    if (mode != "smart" && mode != "global") return "未知分流模式";
    return storeFile(directory + "/routing-mode", mode);
}
std::string Core::serviceChoices() {
    const auto choices = readFile(directory + "/service-choices");
    return choices.empty() ? "fffff" : choices;
}
std::string Core::setServiceChoices(std::string choices) {
    if (choices.size() != 5 || choices.find_first_not_of("fdp") != std::string::npos)
        return "无效的服务规则";
    return storeFile(directory + "/service-choices", choices);
}
std::string Core::manage(std::string request) {
    const std::string offline = R"({"error":"请先连接 VPN"})";
    const std::string failure = R"({"error":"后台响应中断，请重试"})";
    if (request.size() >= 16384) return R"({"error":"输入内容过长"})";
    const int fd = socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
    if (fd < 0) return failure;
    sockaddr_un address{};
    address.sun_family = AF_UNIX;
    const char* path = "/scratch/iori-mihomo/management.sock";
    std::strncpy(address.sun_path, path, sizeof(address.sun_path)-1);
    timeval timeout{2,0};
    setsockopt(fd,SOL_SOCKET,SO_RCVTIMEO,&timeout,sizeof(timeout));
    setsockopt(fd,SOL_SOCKET,SO_SNDTIMEO,&timeout,sizeof(timeout));
    if (::connect(fd,reinterpret_cast<const sockaddr*>(&address),sizeof(address))) {close(fd);return offline;}
    request += "\n";
    size_t sent=0;
    while (sent<request.size()) {
        auto n=send(fd,request.data()+sent,request.size()-sent,MSG_NOSIGNAL);
        if(n<0 && errno==EINTR) continue;
        if(n<=0){close(fd);return failure;}
        sent+=static_cast<size_t>(n);
    }
    std::string reply;
    char buf[4096];
    while(reply.size()<4*1024*1024) {
        auto n=recv(fd,buf,sizeof(buf),0);
        if(n<0 && errno==EINTR) continue;
        if(n<=0) break;
        reply.append(buf,n);
        if(reply.back()=='\n') break;
    }
    close(fd);
    return !reply.empty() && reply.back()=='\n' ? reply : failure;
}
std::string Core::memorySample() {
#ifdef MIHOMO_MEASURE_MEMORY
    auto value = [](const char* path, const char* key) -> long {
        std::ifstream in(path);
        std::string line;
        while (std::getline(in, line))
            if (line.compare(0, strlen(key), key) == 0)
                return strtol(line.c_str()+strlen(key), nullptr, 10);
        return -1;
    };
    return std::to_string(value("/proc/self/smaps_rollup","Pss:")) + " " +
           std::to_string(value("/proc/self/status","VmRSS:"));
#else
    return "";
#endif
}
std::string Core::routeReport() { return readFile(directory + "/route-report"); }

std::string Core::startCore() {
    std::lock_guard<std::mutex> guard(mutex);
    if (active) return "";
    if (access(config.c_str(),R_OK) && access(candidate.c_str(),R_OK)) return "请先导入订阅";
    if (!library) {
        library = dlopen("/pkg/lib/armv7/libtvcore.so", RTLD_NOW | RTLD_LOCAL);
        if (!library) return std::string("无法加载核心：") + dlerror();
        start = reinterpret_cast<Start>(dlsym(library, "TVCoreStart"));
        stop = reinterpret_cast<Stop>(dlsym(library, "TVCoreStop"));
        startLocal = reinterpret_cast<StartLocal>(dlsym(library, "TVLocalWGStart"));
        stopLocal = reinterpret_cast<Stop>(dlsym(library, "TVLocalWGStop"));
    }
    if (!start || !stop || !startLocal || !stopLocal) return "核心库缺少启动或停止接口";
    storeFile(serviceState, "connecting");
    bound = proxyHost();
    if (bound.empty()) return "请连接 Wi-Fi 或有线网络";
    const int error = start(bound.c_str());
    if (error) {
        if ((error == 3 || error == 6) && unlink(candidate.c_str()) && errno!=ENOENT) return strerror(errno);
        const std::string reason = error == 6 ? "订阅未返回可用节点，请检查订阅链接或网络" :
            "启动 Mihomo 失败（" + std::to_string(error) + "）";
        storeFile(serviceState, reason);
        return reason;
    }
    WgKey localPrivate{}, localPublic{}, peerPublic{};
    std::string vpnError;
    if (startLocal(localPrivate.data(),localPublic.data(),peerPublic.data())) {
        vpnError = "启动本地 WireGuard 转发失败";
    } else {
        // 0.4.0 promoted config.json only after the first successful connection.
        const bool legacyProfile = access(ownerFile.c_str(), F_OK) != 0 && errno == ENOENT &&
                                   access(config.c_str(), R_OK) == 0;
        vpnError = prepareSystemVpn(readFile(ownerFile), legacyProfile);
        if (vpnError.empty()) {
            // Persist only the public identity, before the asynchronous create request.
            vpnError = storeFile(ownerFile, std::string(reinterpret_cast<const char*>(peerPublic.data()), peerPublic.size()));
        }
        if (vpnError.empty()) vpnError = startSystemVpn(localPrivate,localPublic,peerPublic);
    }
    explicit_bzero(localPrivate.data(),localPrivate.size());
    if (!vpnError.empty()) {
        stopLocal();
        stop();
        storeFile(serviceState,vpnError);
        return vpnError;
    }
    if (rename(candidate.c_str(),config.c_str()) && errno != ENOENT) {
        const std::string reason = std::string("保存新订阅失败：")+strerror(errno);
        stopSystemVpn(); stopLocal(); stop(); storeFile(serviceState,reason);
        return reason;
    }
    active = true;
    return storeFile(serviceState, "connected");
}
std::string Core::stopCore() {
    std::lock_guard<std::mutex> guard(mutex);
    if (active) {
        const auto error = stopSystemVpn();
        if (!error.empty()) return error;
        stopLocal();
        stop();
    } else if (readFile(serviceState) == "connected") {
        const auto error = prepareSystemVpn(readFile(ownerFile), false);
        if (!error.empty()) {
            storeFile(serviceState, error);
            return error;
        }
    }
    active = false;
    return storeFile(serviceState, "disconnected");
}
std::string Core::coreState() {
    std::lock_guard<std::mutex> guard(mutex);
    if (access(config.c_str(),R_OK) && access(candidate.c_str(),R_OK)) return "unconfigured";
    const std::string actual = readFile(serviceState);
    if (actual == "connected") {
        const auto noBackend = requested() ? "recovering" : "orphaned";
        const int fd = socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
        if (fd < 0) return noBackend;
        sockaddr_un address{};
        address.sun_family = AF_UNIX;
        const char* path="/scratch/iori-mihomo/vpn-ready.sock";
        std::strncpy(address.sun_path,path,sizeof(address.sun_path)-1);
        timeval timeout{0,300000};
        setsockopt(fd,SOL_SOCKET,SO_RCVTIMEO,&timeout,sizeof(timeout));
        unsigned char ready = 0;
        const bool connected = !::connect(fd,reinterpret_cast<const sockaddr*>(&address),sizeof(address)) &&
                               ::recv(fd,&ready,1,0)==1 && ready==1;
        close(fd);
        return connected ? "running" : noBackend;
    }
    if (!actual.empty() && actual!="connected" && actual!="disconnected" && actual!="connecting") return actual;
    return requested() ? "starting" : "stopped";
}
std::string Core::saveConfig(std::string json) {
    std::lock_guard<std::mutex> guard(mutex);
    if (requested()) return "请先断开连接";
    return storeFile(candidate, json);
}
}
