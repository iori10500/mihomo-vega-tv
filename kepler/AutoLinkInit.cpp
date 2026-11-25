#include <Kepler/turbomodule/KeplerTurboModuleRegistration.h>
#include "Core.h"
extern "C" __attribute__((visibility("default"))) void autoLinkKeplerTurboModulesV1() noexcept {
    KEPLER_REGISTER_TURBO_MODULE(mihomotv, Core);
}
