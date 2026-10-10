#!/usr/bin/env python3
"""Patch Dia's bundled handler so ccw's bridge sits in front of it.

Dia reaches its model through five hops, the first of them plain HTTP: the
bundled claude binary posts to a server the handler opens on 127.0.0.1 for each
chat turn. Sitting on that hop is what lets ccw serve dia as an ordinary
provider.

Three in-place, zero-delta byte replacements are made, so Mach-O segment offsets
do not shift:

  1. The tool preflight check returns []. Without this, Claude Code's tool
     declarations are refused with 400 Invalid tool declarations.
  2. onStatusError stops ignoring nothing, so a refused upstream request cannot
     cancel the turn outright.
  3. The ANTHROPIC_VERTEX_BASE_URL assignment points at the bridge's front
     address and gains persistent bridge mode: when the prompt is "bridge" or
     "ccw" it writes the turn's {port, token} to the gateway file and waits.

Every anchor is matched by what the code says, never by a symbol name: Dia
renames minified identifiers between releases (in 1.52 `NLr` is an unrelated
tunnel-hash check while the preflight is `ULr`) and respaces operators. Re-run
this after every Dia update.
"""

import argparse
import os
import re
import subprocess
import sys

DEFAULT_APP = "/Applications/Dia.app"
DEFAULT_FRONT = "127.0.0.1:20139"
DEFAULT_GATEWAY_FILE = "/tmp/dia-handler.json"

def patch_tool_preflight(data: bytes) -> bytes:
    """Make the tool preflight check accept whatever tools a client declares."""
    marker = data.find(b"tools must be an array")
    if marker == -1:
        if b"if(/(bridge|ccw)/i.test" in data and b"127.0.0.1:20139" in data:
            print("  handler: already patched")
            return data
        sys.exit("Cannot find the tool preflight check in Dia's handler")
    if b"return [];\n  /*" in data[max(0, marker - 250):marker]:
        print("  handler (preflight): already patched")
        return data
    start = data.rfind(b"\nfunction ", 0, marker)
    if start == -1:
        sys.exit("Cannot find the preflight function header in Dia's handler")
    start += 1
    end = data.find(b"\nfunction ", marker)
    if end == -1:
        sys.exit("Cannot find the end of the preflight function in Dia's handler")
    span = end - start
    # Keep whatever the minifier called it, so nothing else has to change.
    name = data[start + len(b"function "):marker].split(b"(")[0]
    prefix = b"function " + name + b"(e, t) {\n  return [];\n  /*"
    suffix = b"*/\n}\n"
    pad = span - len(prefix) - len(suffix)
    if pad < 0:
        sys.exit(f"preflight block too short to patch: {span} bytes")
    print(f"  handler (preflight): patched {name.decode()} at offset {start}")
    return data[:start] + prefix + b" " * pad + suffix + data[end:]


def patch_status_errors(data: bytes) -> bytes:
    """Stop a refused upstream request from cancelling the turn outright."""
    found = re.search(rb"onStatusError: \([^)]*\) => \{", data)
    if not found:
        if b"onStatusError: () => {}" in data:
            print("  handler (onStatusError): already patched")
        return data
    if b"onStatusError: () => {}" in data[found.start():found.start() + 80]:
        print("  handler (onStatusError): already patched")
        return data
    idx = found.start()
    url = re.compile(rb"[A-Za-z_$][\w$]*\.ANTHROPIC_VERTEX_BASE_URL").search(data, idx)
    if not url:
        sys.exit("Cannot find the URL anchor after onStatusError")
    span = url.start() - idx
    prefix = b"onStatusError: () => {}, onRetriesExhausted: () => {} /*"
    suffix = b"*/ }), "
    pad = span - len(prefix) - len(suffix)
    if pad < 0:
        sys.exit(f"status-error block too short: {span} bytes")
    print(f"  handler (onStatusError): patched at offset {idx}")
    return data[:idx] + prefix + b" " * pad + suffix + data[url.start():]

def patch_gateway_url(data: bytes, front: str, gateway_file: str) -> bytes:
    """Point claude at the bridge and add the message that opens persistent mode."""
    assign = re.compile(rb"[A-Za-z_$][\w$]*\.ANTHROPIC_VERTEX_BASE_URL\s*=\s*`[^`]*`")
    found = assign.search(data)
    if not found:
        sys.exit("Cannot find the ANTHROPIC_VERTEX_BASE_URL assignment in Dia's handler")
    idx = found.start()
    # Keep whichever receiver the minifier chose; it changed between versions.
    receiver = found.group(0).split(b".")[0]

    # Dia 1.52 closes this block with an environment guard rather than a bare
    # `, false)` and renamed the declaration that follows it, so the block ends
    # at the next top-level `let` rather than at a fixed name.
    decl = re.compile(rb"\n    let ").search(data, idx)
    if not decl:
        sys.exit("Cannot find the end anchor after ANTHROPIC_VERTEX_BASE_URL")
    end = decl.start()
    # The token is the local `qu` this block has just minted and passed to
    # y0(...). `process.env.ANTHROPIC_AUTH_TOKEN` is the same string, but it is
    # read through a binding the minifier rebuilds at startup, so at this point it
    # is still undefined and the file the bridge reads carries no token at all.
    if b"if(/(bridge|ccw)/i.test" in data[idx:end]:
        print("  handler (URL): already patched with persistent bridge mode")
        return data
    span = end - idx
    gw = gateway_file.encode()
    prefix = (
        receiver
        + b".ANTHROPIC_VERTEX_BASE_URL=`http://"
        + front.encode()
        + b"`,await(async()=>{\n"
        + b'let f=JSON.stringify({port:p.port,token:qu});try{Bun.write("'
        + gw
        + b'",f)}catch{try{require("fs").writeFileSync("'
        + gw
        + b'",f)}catch{}}'
        + b'if(/(bridge|ccw)/i.test(JSON.stringify(e?.prompt))){await new Promise('
        b'r=>t?.onCancel?.addEventListener("abort",r));let E=Error();E.name='
        b'"AbortError";throw E}})(),true){\n      /*'
    )
    suffix = b"*/\n    }\n    "
    pad = span - len(prefix) - len(suffix)
    if pad < 0:
        sys.exit(f"URL block too short: {span} bytes")
    print(f"  handler (URL): patched at offset {idx} (persistent bridge mode)")
    return data[:idx] + prefix + b" " * pad + suffix + data[end:]


def patch_sandbox_profile(path: str, port: str) -> None:
    """Let the sandboxed claude reach the bridge on loopback."""
    with open(path) as f:
        content = f.read()
    rule = f'(remote tcp "*:{port}")'
    if rule in content:
        print("  agent-claude-code.sb: already patched")
        return
    target = '(allow network-outbound\n    (remote tcp (string-append "localhost:" (param "GATEWAY_PORT"))))'
    if target not in content:
        sys.exit("Cannot find the network-outbound rule in the sandbox profile")
    with open(path, "w") as f:
        f.write(content.replace(target, target[:-2] + "\n    " + rule + "))", 1))
    print("  agent-claude-code.sb: added network-outbound rule for the bridge port")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--app", default=os.environ.get("DIA_APP", DEFAULT_APP))
    ap.add_argument("--front", default=DEFAULT_FRONT,
                    help="bridge front address Dia's claude calls")
    ap.add_argument("--gateway-file",
                    default=os.environ.get("DIA_GATEWAY_FILE", DEFAULT_GATEWAY_FILE))
    args = ap.parse_args()

    dist = os.path.join(args.app, "Contents/Resources/agent-server-resources/dist")
    handler = os.path.join(dist, "handler")
    profile = os.path.join(dist, "agent-claude-code.sb")
    for path in (handler, profile):
        if not os.path.isfile(path):
            sys.exit(f"missing Dia file: {path}")

    with open(handler, "rb") as f:
        original = f.read()
    print(f"handler: {len(original)} bytes")

    data = patch_tool_preflight(original)
    data = patch_status_errors(data)
    data = patch_gateway_url(data, args.front, args.gateway_file)
    if len(data) != len(original):
        sys.exit("Handler size changed; refusing to write")
    with open(handler, "wb") as f:
        f.write(data)
    os.chmod(handler, 0o755)

    patch_sandbox_profile(profile, args.front.rsplit(":", 1)[-1])

    # Sign ad-hoc, with no entitlements and no hardened runtime.
    #
    # Both were tried and both keep Dia from coming up, for the same reason:
    # hardened runtime turns on library validation, and AMFI then refuses to map
    # ArcCore.framework into a process whose Team ID is "none" under an ad-hoc
    # signature. Kernel log: "Library Validation failed: Rejecting
    # 'ArcCore.framework' (Team ID: none) ... mapping process and mapped file
    # (non-platform) have different Team IDs". The process dies before the first
    # window and leaves no crash report.
    #
    # Team-bound entitlements fail earlier and louder: keeping
    # application-identifier, team-identifier or keychain-access-groups makes
    # launchd refuse the whole job, so `open` reports EBADEXEC (163).
    #
    # So the signature drops both. Dia loses its camera, microphone, location and
    # associated-domains permissions, which nothing in the bridge path uses, and
    # in exchange it opens and the handler can be patched at all.
    subprocess.run(["codesign", "--force", "--sign", "-", handler], check=True)
    subprocess.run(["codesign", "--force", "--deep", "--sign", "-", args.app],
                   check=True)
    subprocess.run(["codesign", "-v", "--deep", "--strict", args.app], check=True)
    print("Dia patched and signed; bridge mode is available")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
