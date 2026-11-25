"""Collect redistributable software notices for the packages used by the TV build."""
import collections
import json
from pathlib import Path
import re
import subprocess
import sys

project = Path(sys.argv[1])
dest = project / "assets/raw/Third-party-notices.txt"
notice_name = re.compile(r"^(?:licen[cs]es?|copying|unlicense|copyright|notice|third.party.notices)(?:$|[._-])", re.I)
texts = collections.defaultdict(set)


def collect(directory, label):
    for f in sorted(directory.iterdir()):
        if f.is_file() and notice_name.match(f.name):
            text = f.read_bytes().decode("utf-8", errors="replace").strip()
            if text:
                texts[text].add(label + " / " + f.name)


def go(command):
    return subprocess.check_output(["go", *command], text=True)


collect(Path(go(["env", "GOROOT"]).strip()), "Go " + go(["env", "GOVERSION"]).strip())
records = go(["list", "-mod=mod", "-tags", "netgo osusergo", "-deps", "-json=Dir,Module", "."])
decoder = json.JSONDecoder()
visited = set()
while records.strip():
    package, consumed = decoder.raw_decode(records.lstrip())
    records = records.lstrip()[consumed:]
    module = package.get("Module")
    if not module or module.get("Main"):
        continue
    source = module.get("Replace", module)
    root = Path(source["Dir"]).resolve()
    current = Path(package["Dir"]).resolve()
    version = source.get("Version") or ("v1.19.34" if module["Path"] == "github.com/metacubex/mihomo" else "")
    label = source["Path"] if source.get("Version") else module["Path"]
    while current == root or root in current.parents:
        if current not in visited:
            relative = current.relative_to(root).as_posix()
            collect(current, (label + " " + version + ("/" + relative if relative != "." else "")).strip())
            visited.add(current)
        if current == root:
            break
        current = current.parent

deps = json.loads((project / "package.json").read_text())["dependencies"]
# Additional modules present in the Metro source maps for the locked production bundles.
bundled_js = """
@amazon-devices/kepler-compatibility-metro-config @babel/runtime
@react-native/assets-registry @react-native/js-polyfills @react-native/normalize-colors
@react-native/virtualized-lists abort-controller base64-js deprecated-react-native-prop-types
event-target-shim invariant memoize-one nullthrows promise prop-types regenerator-runtime
scheduler stacktrace-parser use-sync-external-store whatwg-fetch
""".split()
for name in sorted(set(deps) | set(bundled_js)):
    root = project / "node_modules" / name
    meta = json.loads((root / "package.json").read_text())
    label = name + " " + meta["version"]
    collect(root, label)
    # These MIT packages refer to the license at their React Native source repository root.
    if name.startswith("@react-native/") or name == "deprecated-react-native-prop-types":
        license_file = project / "node_modules/react-native/LICENSE"
        if not (root / "LICENSE").exists() and meta.get("license") == "MIT":
            texts[license_file.read_text().strip()].add(label + " / React Native MIT license")

with dest.open("w", encoding="utf-8") as out:
    out.write("Mihomo TV — software notices\nSource and build scripts: https://github.com/iori10500/mihomo-vega-tv\n\n")
    for text, labels in sorted(texts.items(), key=lambda item: sorted(item[1])):
        out.write("\n".join(sorted(labels)) + "\n\n" + text + "\n\n")
