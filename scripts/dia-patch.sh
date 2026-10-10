#!/usr/bin/env bash
# Point Dia's bundled claude at ccw's dia bridge.
#
# Dia reaches its model through five hops. The first is plain HTTP: the bundled
# claude binary posts to a handler-local server on 127.0.0.1, which the handler
# opens fresh for every chat turn and protects with a token it generates there.
# Sitting on that hop is what lets ccw serve dia as an ordinary provider: the
# bridge forwards Dia's own traffic to whatever gateway the current turn built,
# and dials that gateway for ccw with the credential of that same turn.
#
# Three patches are applied:
#   1. handler (Mach-O Bun binary):
#      - NLr: disabled tool preflight check (returns []) so Claude Code CLI tools
#        (Bash, Edit, Glob, etc.) are not rejected with 400 Invalid tool declarations.
#      - kt.ANTHROPIC_VERTEX_BASE_URL: set to http://127.0.0.1:20139 and enable
#        persistent bridge mode when prompt is 'bridge' or 'ccw', writing
#        { port: p.port, token: tc } to /tmp/dia-handler.json.
#      Both are in-place exact-byte replacements (0-byte delta) so Mach-O segment
#      offsets do not shift.
#   2. agent-claude-code.sb (Seatbelt profile):
#      Allow outbound TCP to localhost:20139 so the sandbox does not block claude.
#
# Usage: sudo bash scripts/dia-patch.sh [--front ADDR] [--port-file PATH]
set -euo pipefail

APP="${DIA_APP:-/Applications/Dia.app}"
HANDLER="$APP/Contents/Resources/agent-server-resources/dist/handler"
SB="$APP/Contents/Resources/agent-server-resources/dist/agent-claude-code.sb"
GATEWAY_FILE="${DIA_GATEWAY_FILE:-/tmp/dia-handler.json}"
FRONT="127.0.0.1:20139"

[ -f "$HANDLER" ] || { echo "no Dia handler at $HANDLER" >&2; exit 1; }
[ -f "$SB" ] || { echo "no sandbox profile at $SB" >&2; exit 1; }

python3 - <<EOF
import os, re, subprocess

handler_path = "$HANDLER"
sb_path = "$SB"
front = "$FRONT"
gw_file = "$GATEWAY_FILE"

data = open(handler_path, 'rb').read()
orig_len = len(data)

# 1. Patch NLr (tool preflight disable)
#
# The signature's parameter names are minified differently between Dia versions,
# so only the function name is matched; the block ends at the next top-level
# `function`.
nlr_re = re.compile(rb'function NLr\([^)]*\) \{')
m_nlr = nlr_re.search(data)
assert m_nlr, "Cannot find function NLr anchor"
start_nlr = m_nlr.start()
end_nlr = data.find(b'\nfunction ', start_nlr + 1)
if end_nlr == -1:
    end_nlr = start_nlr + len(m_nlr.group())
end_nlr = data.find(b'{', start_nlr + len(m_nlr.group()))
end_nlr = data.find(b'\nfunction ', end_nlr) if data.find(b'\nfunction ', end_nlr) != -1 else len(data)
if b'function NLr(' in data[start_nlr:start_nlr + 40] and b'return [];' in data[start_nlr:start_nlr + 60]:
    print("  handler (NLr): already patched")
else:
    orig_nlr_len = end_nlr - start_nlr
    prefix_nlr = b'function NLr(e, t) {\n  return [];\n  /*'
    suffix_nlr = b'*/\n}\n'
    padding_nlr = b' ' * (orig_nlr_len - len(prefix_nlr) - len(suffix_nlr))
    replacement_nlr = prefix_nlr + padding_nlr + suffix_nlr
    assert len(replacement_nlr) == orig_nlr_len, f"NLr block too short: {orig_nlr_len}"
    data = data[:start_nlr] + replacement_nlr + data[end_nlr:]
    print(f"  handler (NLr): patched at offset {start_nlr} (tools allowed)")
# 1.5. Patch onStatusError (prevent upstream 4xx errors from interrupting bridge)
idx_err = data.find(b"onStatusError: (Ie, \$t, Rn, Ii) => {")
if idx_err == -1:
    m_err = re.search(rb'onStatusError: \([^)]*\) => \{', data)
    idx_err = m_err.start() if m_err else -1
if idx_err != -1:
    m_url = re.compile(rb'[A-Za-z_$][\w$]*\.ANTHROPIC_VERTEX_BASE_URL').search(data, idx_err)
    assert m_url, "Cannot find URL anchor after onStatusError"
    idx_url = m_url.start()
    err_len = idx_url - idx_err
    prefix_err = b"onStatusError: () => {}, onRetriesExhausted: () => {} /*"
    suffix_err = b"*/ }), "
    pad_err = err_len - len(prefix_err) - len(suffix_err)
    assert pad_err >= 0, f"Target error block too short: {err_len}"
    rep_err = prefix_err + (b" " * pad_err) + suffix_err
    assert len(rep_err) == err_len
    data = data[:idx_err] + rep_err + data[idx_url:]
    print("  handler (onStatusError): patched to ignore errors")
elif b"onStatusError: () => {}" in data:
    print("  handler (onStatusError): already patched")


# 2. Patch ANTHROPIC_VERTEX_BASE_URL (persistent bridge mode)
#
# Dia 1.52 renamed the receiver from `kt` to `hr` and put spaces around the `=`,
# so the anchor is matched by name and tolerates either spacing. Matching the
# variable name rather than the whole statement is what survives a version bump.
url_re = re.compile(rb'[A-Za-z_$][\w$]*\.ANTHROPIC_VERTEX_BASE_URL\s*=\s*`[^`]*`')
m = url_re.search(data)
assert m, "Cannot find ANTHROPIC_VERTEX_BASE_URL assignment"
idx = m.start()
recv = m.group(0).split(b'.')[0]
end = data.find(b'let cl = process.env.DIA_AGENTSERVER_ALTERNATE_DESKTOP_NAME', idx)
assert end != -1, "Cannot find the end anchor after ANTHROPIC_VERTEX_BASE_URL"
if b'if(/(bridge|ccw)/i.test' in data[idx:end]:
    print("  handler (URL): already patched with persistent bridge mode")
else:
    orig_url_len = end - idx
    p1 = recv + b'.ANTHROPIC_VERTEX_BASE_URL=\`http://' + front.encode() + b'\`,await(async()=>{\n'
    p2 = b'let f=JSON.stringify({port:p.port,token:tc});try{Bun.write("' + gw_file.encode() + b'",f)}catch{try{require("fs").writeFileSync("' + gw_file.encode() + b'",f)}catch{}}'
    p3 = b'if(/(bridge|ccw)/i.test(JSON.stringify(e?.prompt))){await new Promise(r=>t?.onCancel?.addEventListener("abort",r));let E=Error();E.name="AbortError";throw E}})(),false){\n      /*'
    prefix_url = p1 + p2 + p3
    suffix_url = b'*/\n    }\n    '
    needed_padding = orig_url_len - len(prefix_url) - len(suffix_url)
    assert needed_padding >= 0, f"Target block too short: {orig_url_len}"
    replacement_url = prefix_url + (b' ' * needed_padding) + suffix_url
    assert len(replacement_url) == orig_url_len
    data = data[:idx] + replacement_url + data[end:]
    print(f"  handler (URL): patched at offset {idx} (persistent bridge mode)")
assert len(data) == orig_len, "Handler size changed!"
open(handler_path, 'wb').write(data)
os.chmod(handler_path, 0o755)

# Sign handler directly
subprocess.run(["codesign", "--force", "--sign", "-", handler_path], check=True)

# 3. Patch agent-claude-code.sb
sb_content = open(sb_path, 'r').read()
rule = f'(remote tcp "*:{front.split(":")[-1]}")'
if rule in sb_content:
    print("  agent-claude-code.sb: already patched")
else:
    target = '(allow network-outbound\n    (remote tcp (string-append "localhost:" (param "GATEWAY_PORT"))))'
    replacement_sb = f'(allow network-outbound\n    (remote tcp (string-append "localhost:" (param "GATEWAY_PORT")))\n    (remote tcp "*:{front.split(":")[-1]}"))'
    assert target in sb_content, "Cannot find network-outbound target in sandbox profile"
    open(sb_path, 'w').write(sb_content.replace(target, replacement_sb, 1))
    print("  agent-claude-code.sb: added network-outbound rule for bridge port")

# 4. Sign entire bundle
subprocess.run(["codesign", "--force", "--deep", "--sign", "-", "$APP"], check=True)
EOF

codesign -v --deep --strict "$APP" && echo "signed $APP (ad-hoc valid)"
echo "Dia fully patched and ready!"
