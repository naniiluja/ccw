# What the Zen gateway sent

`<case>.golden.json` is the request that zencore (the Python gateway that
`opencode` replaces) sent upstream for the input in `cases.json`, recorded on
2026-09-30 by pointing it at a local recorder. The session, project and
prompt-cache ids are masked.

`TestZenRequestsMatchTheGatewayItReplaces` sends the same inputs through ccw
and holds it to the same path, the same identity headers and the same body. One
difference is on purpose and the test names it: ccw drops the caller's `user`
field, which for Claude Code holds a device id.

To record a case again, run zencore with `--upstream` pointing at a server that
logs what it gets and answers with a canned stream, send it the input, and mask
the ids.
