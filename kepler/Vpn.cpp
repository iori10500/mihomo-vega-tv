#include "Vpn.h"
#include <apmf/data_object.h>
#include <apmf/process.h>
#include <apmf/iface/com/amazon/kepler/net/vpn/vpnmanager/IVpnManager.h>
#include <apmf/iface/com/amazon/kepler/net/vpn/wireguard/WgVpnConfig.h>
#include <apmf/iface/com/amazon/kepler/net/vpn/wireguard/WgInterfaceConfig.h>
#include <apmf/iface/com/amazon/kepler/net/vpn/wireguard/WgPeerConfig.h>
#include <apmf/iface/com/amazon/kepler/net/vpn/wireguard/WgConfig.h>
#include <chrono>
#include <apmf/apmf_base.h>
#include <apmf/iface/com/amazon/kepler/net/vpn/vpnmanager/IVpnEventListener.h>
#include <apmf/iface/com/amazon/kepler/net/vpn/common/VpnEventData.h>
#include <mutex>
#include <condition_variable>
#include <thread>

namespace mihomotv {
namespace {
namespace wg = apmf::iface::com::amazon::kepler::net::vpn::wireguard;
namespace systemvpn = apmf::iface::com::amazon::kepler::net::vpn::common;
using VpnManager = apmf::iface::com::amazon::kepler::net::vpn::vpnmanager::IVpnManager;
using Event = systemvpn::VpnCallbackEvent;
using EventIface = apmf::iface::com::amazon::kepler::net::vpn::vpnmanager::IVpnEventListener;
class Events : public apmf::ApmfBase<Events, EventIface> {
public:
    void onVpnEventCb(Event event,apmf::View<systemvpn::VpnEventData> data) {
        std::lock_guard<std::mutex> lock(mutex);
        seen |= 1u << static_cast<int>(event);
        if(data) reason = static_cast<int>(data->reasonCode());
        changed.notify_all();
    }
    bool wait(Event desired,Event failure) {
        const auto mask=1u << static_cast<int>(desired);
        const auto bad=1u << static_cast<int>(failure);
        std::unique_lock<std::mutex> lock(mutex);
        if(!changed.wait_for(lock,std::chrono::seconds(8),[&]{return seen & (mask|bad);})) return false;
        if(seen & bad) throw std::runtime_error("系统返回 "+std::to_string(reason));
        return true;
    }
private:
    std::mutex mutex;
    std::condition_variable changed;
    unsigned seen=0;
    int reason=0;
};
apmf::Ptr<VpnManager> manager;
apmf::Ptr<Events> events;
bool ownsInterface = false;
}
std::string stopSystemVpn() {
    if (!manager) return "";
    try {
        if (ownsInterface) {
            manager->stopVpn();
            ownsInterface=false;
        }
        if(events) {manager->removeVpnEventListener(); events=nullptr;}
        return "";
    } catch (const std::exception& e) {return std::string("停止系统 VPN 失败：")+e.what();}
}
std::string startSystemVpn(const WgKey& localPrivate,const WgKey& localPublic,const WgKey& peerPublic) {
    const char* step = "获取 VPN 系统接口";
    try {
        manager=apmf::GetProcessObject()->getComponent("/com.amazon.kepler.net.vpn.vpnmanager").QueryInterface<VpnManager>();
        step = "读取 VPN 配置";
        if (manager->getVpnConfig()) return "电视上已有活动的 VPN 配置";
        step = "订阅 VPN 连接事件";
        auto listener = apmf::Ptr<Events>::Make();
        manager->addVpnEventListener(listener);
        events=listener;
        step = "配置本地 WireGuard 接口";
        auto iface=apmf::MakeBuilderFor<wg::WgInterfaceConfig>();
        iface->ip("10.203.77.2/32");
        iface->privKey({localPrivate.data(),localPrivate.size()});
        iface->pubKey({localPublic.data(),localPublic.size()});
        iface->listenPort(0);
        auto peer=apmf::MakeBuilderFor<wg::WgPeerConfig>();
        peer->pubKey({peerPublic.data(),peerPublic.size()});
        peer->allowedIps({"0.0.0.0/0"});
        peer->epType(wg::WgEndpointType::IP);
        peer->epIp("127.0.0.1");
        peer->listenPort(17892);
        peer->keepAlive(15);
        auto settings=apmf::MakeBuilderFor<wg::WgConfig>();
        settings->ifConfig(iface->build().QueryInterface<wg::WgInterfaceConfig>());
        settings->peerConfigs({peer->build().QueryInterface<wg::WgPeerConfig>()});
        auto vpn=apmf::MakeBuilderFor<wg::WgVpnConfig>();
        vpn->vpnType(systemvpn::VpnType::WG);
        vpn->wgConfig(settings->build().QueryInterface<wg::WgConfig>());
        step = "创建 VPN";
        manager->createVpn(vpn->build().QueryInterface<wg::WgVpnConfig>());
        ownsInterface=true;
        step = "等待 VPN 接口";
        if (!events->wait(Event::IF_UP,Event::CREATE_FAILURE)) throw std::runtime_error("接口未就绪");
        step = "设置 VPN 地址";
        manager->addInterfaceAddress("10.203.77.2/32");
        step = "设置 VPN 路由";
        manager->addRoute("0.0.0.0/0");
        manager->addDnsServer("1.1.1.1");
        manager->setMtu(1280);
        step = "启动 VPN";
        manager->startVpn();
        if (!events->wait(Event::CONNECTED,Event::START_FAILURE)) throw std::runtime_error("系统未连接");
        return "";
    } catch (const std::exception& e) {
        auto rollback=stopSystemVpn();
        return std::string(step)+"："+e.what()+(rollback.empty()?"":"；"+rollback);
    }
}
}
