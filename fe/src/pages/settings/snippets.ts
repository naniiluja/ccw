// Connection snippets. The Claude Code block follows docs/clients.md; the other
// two use the same base URL and an API key made on the API keys page.

export interface Snippet {
  id: string
  title: string
  /** Where the text goes, shown above the block. */
  hint: string
  language: string
  code: string
}

export function buildSnippets(origin: string): Snippet[] {
  return [
    {
      id: 'claude-code',
      title: 'Claude Code',
      hint: 'Thêm vào khối "env" của ~/.claude/settings.json. Địa chỉ gốc không có /v1, Claude Code tự thêm /v1/messages.',
      language: 'json',
      code: `{
  "env": {
    "ANTHROPIC_BASE_URL": "${origin}",
    "ANTHROPIC_AUTH_TOKEN": "<khóa API của ccw>",
    "ANTHROPIC_MODEL": "antigravity/claude-sonnet-4-6",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "antigravity/claude-opus-4-6-thinking",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "antigravity/claude-sonnet-4-6",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "antigravity/gemini-3.7-flash"
  }
}`,
    },
    {
      id: 'codex',
      title: 'Codex',
      hint: 'Thêm vào ~/.codex/config.toml, rồi đặt biến môi trường CCW_API_KEY bằng khóa API.',
      language: 'toml',
      code: `model_provider = "ccw"
model = "antigravity/claude-sonnet-4-6"

[model_providers.ccw]
name = "ccw"
base_url = "${origin}/v1"
env_key = "CCW_API_KEY"
wire_api = "responses"`,
    },
    {
      id: 'openai',
      title: 'Client tương thích OpenAI',
      hint: 'Đặt base URL và khóa API; ví dụ liệt kê model bằng curl.',
      language: 'bash',
      code: `export OPENAI_BASE_URL="${origin}/v1"
export OPENAI_API_KEY="<khóa API của ccw>"

curl "$OPENAI_BASE_URL/models" \\
  -H "Authorization: Bearer $OPENAI_API_KEY"`,
    },
  ]
}
