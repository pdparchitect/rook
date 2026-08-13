# Providers

A run targets a **provider** - the model provider Rook talks to. Rook speaks to
each one directly over the OpenAI-compatible API; there is no gateway and no
account in between, so all you need is a provider key. Pick a provider with
`--provider`, or set `default_provider` in config.

| Provider     | Endpoint                         | Credential from      |
| ------------ | -------------------------------- | -------------------- |
| `zai`        | `https://api.z.ai/api/paas/v4`   | `ZAI_API_KEY`        |
| `openai`     | `https://api.openai.com/v1`      | `OPENAI_API_KEY`     |
| `anthropic`  | `https://api.anthropic.com/v1`   | `ANTHROPIC_API_KEY`  |
| `groq`       | `https://api.groq.com/openai/v1` | `GROQ_API_KEY`       |
| `mistral`    | `https://api.mistral.ai/v1`      | `MISTRAL_API_KEY`    |
| `deepseek`   | `https://api.deepseek.com/v1`    | `DEEPSEEK_API_KEY`   |
| `openrouter` | `https://openrouter.ai/api/v1`   | `OPENROUTER_API_KEY` |
| `together`   | `https://api.together.xyz/v1`    | `TOGETHER_API_KEY`   |
| `cerebras`   | `https://api.cerebras.ai/v1`     | `CEREBRAS_API_KEY`   |
| `xai`        | `https://api.x.ai/v1`            | `XAI_API_KEY`        |
| `moonshot`   | `https://api.moonshot.cn/v1`     | `MOONSHOT_API_KEY`   |
| `qwen`       | DashScope compatible mode        | `DASHSCOPE_API_KEY`  |
| `abliteration` | `https://api.abliteration.ai/v1` | `ABLIT_KEY` |
| `ollama`     | `http://localhost:11434/v1`      | none (local)         |

Rook defaults to **`zai`** running **`glm-5.2`** - a strong open model for
bug-hunting work: large context for reading codebases, and permissive for
offensive tasks. The model must be one the chosen provider serves.

The common case is one exported variable and nothing else:

```bash
export ZAI_API_KEY="sk-..."
rook new "Reverse engineer ./firmware.bin and identify remotely reachable bugs"
# edit .rook/objectives/reverse-engineer-firmware-bin-and-identify-.yaml
# to set the success criteria, then:
rook
```

Switch provider with a flag:

```bash
export OPENAI_API_KEY="sk-..."
rook --provider openai --model gpt-5 "…"
```

For sensitive material that must not leave the machine, a local model is the
right choice - and the one provider that never sends data off-host:

```bash
rook --provider ollama --model llama-4 "…"
```

For models that refuse less on offensive-security tasks,
[Abliteration.ai](https://abliteration.ai) hosts less-restrictive variants for
security research over the same OpenAI-compatible API:

```bash
export ABLIT_KEY="sk-..."
rook --provider abliteration --model abliterated-model "…"
```

## Any other provider

Anything that speaks the OpenAI-compatible API works. Name a provider, give it a
base URL and a key:

```yaml
default_provider: mygateway
providers:
  mygateway:
    driver: custom
    base_url: https://gateway.internal.example.com/v1
    api_key: '$GATEWAY_KEY'
```

A key can be written literally or as a `$VAR` reference so no secret is on disk.

A built-in provider's conventional key (e.g. `OPENAI_API_KEY`) is **withheld**
once you set a custom `base_url` on it: a key scoped to the provider's real host
is never forwarded to a URL you typed into the config, so an overridden
connection must carry a key written for it. See [configuration.md](configuration.md)
for how credentials are resolved and scrubbed.
