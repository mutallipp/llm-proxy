# OpenCode Integration Guide

---

## Overview
llm-proxy can act as a drop-in replacement for Anthropic endpoints, letting OpenCode connect through your own infrastructure. This guide explains how to configure OpenCode and how to combine it with llm-proxy model profiles for flexible routing.

### Key Points
- llm-proxy performs AI protocol/format transformation. You can configure multiple upstream channels (providers) and expose a single Anthropic-compatible interface for OpenCode.
- You can aggregate OpenCode requests from the same session into one trace (see "Configure OpenCode").

### Prerequisites
- llm-proxy instance reachable from your development machine.
- Valid llm-proxy API key with project access.
- Access to OpenCode CLI tool.
- Optional: one or more model profiles configured in the llm-proxy console.

---

## Configure OpenCode

### 1. Create OpenCode Configuration File

Create or edit your OpenCode configuration file at `~/.config/opencode/opencode.json`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "plugin": [
    "opencode-llm-proxy-tracing"
  ],
  "provider": {
    "llm-proxy": {
      "npm": "@ai-sdk/anthropic",
      "name": "llm-proxy",
      "options": {
        "baseURL": "http://127.0.0.1:8090/anthropic/v1",
        "apiKey": "LLM_PROXY_API_KEY"
      },
      "models": {
        "claude-sonnet-4-5": {
          "name": "llm-proxy - Claude Sonnet 4.5",
          "modalities": {
            "input": [
              "text",
              "image"
            ],
            "output": [
              "text"
            ]
          }
        }
      }
    }
  }
}
```

### 2. Configuration Parameters

| Parameter | Description | Example |
|-----------|-------------|---------|
| `npm` | The npm package to use for the provider | `@ai-sdk/anthropic` |
| `name` | Display name for the provider | `llm-proxy` |
| `baseURL` | llm-proxy Anthropic API endpoint | `http://127.0.0.1:8090/anthropic/v1` |
| `apiKey` | Your llm-proxy API key | Replace `LLM_PROXY_API_KEY` with your actual key |

### 3. Add Multiple Models

You can configure multiple models in the same provider:

```json
{
  "provider": {
    "llm-proxy": {
      "npm": "@ai-sdk/anthropic",
      "name": "llm-proxy",
      "options": {
        "baseURL": "http://127.0.0.1:8090/anthropic/v1",
        "apiKey": "your-llm-proxy-api-key"
      },
      "models": {
        "claude-sonnet-4-5": {
          "name": "llm-proxy - Claude Sonnet 4.5",
          "modalities": {
            "input": ["text", "image"],
            "output": ["text"]
          }
        },
        "claude-haiku-4-5": {
          "name": "llm-proxy - Claude Haiku 4.5",
          "modalities": {
            "input": ["text", "image"],
            "output": ["text"]
          }
        },
        "claude-opus-4-5": {
          "name": "llm-proxy - Claude Opus 4.5",
          "modalities": {
            "input": ["text", "image"],
            "output": ["text"]
          }
        }
      }
    }
  }
}
```

### 4. Using Remote llm-proxy Instance

If your llm-proxy instance is deployed remotely, update the `baseURL`:

```json
{
  "options": {
    "baseURL": "https://your-llm-proxy-domain.com/anthropic/v1",
    "apiKey": "your-llm-proxy-api-key"
  }
}
```

## Working with Model Profiles

llm-proxy model profiles remap incoming model names to provider-specific equivalents:
- Create a profile in the llm-proxy console and add mapping rules (exact name or regex).
- Assign the profile to your API key.
- Switch active profiles to alter OpenCode behavior without changing tool settings.

<table>
  <tr align="center">
    <td align="center">
      <a href="../../screenshots/llm-proxy-profiles.png">
        <img src="../../screenshots/llm-proxy-profiles.png" alt="Model Profiles" width="250"/>
      </a>
      <br/>
      Model Profiles
    </td>
  </tr>
</table>

### Example Use Cases

#### Cost Optimization
Map expensive models to cheaper alternatives:
- Request `claude-sonnet-4-5` → mapped to `deepseek-chat` for reducing costs
- Request `claude-haiku-4-5` → mapped to `gpt-4o-mini` for simple tasks

#### Performance Optimization
Route to faster models for specific tasks:
- Request `claude-opus-4-5` → mapped to `claude-sonnet-4-5` for faster responses
- Request `claude-sonnet-4-5` → mapped to `gpt-4o` for better availability

#### Advanced Reasoning
Route to specialized models:
- Request `claude-sonnet-4-5` → mapped to `deepseek-reasoner` for complex reasoning tasks
- Request `claude-opus-4-5` → mapped to `o1-preview` for mathematical problems

---

## OpenCode Tracing Plugin

The `opencode-llm-proxy-tracing` plugin injects trace headers for every LLM request, enabling request aggregation and tracing in llm-proxy.

### Default Headers

| Header Key | Source | Description |
|------------|--------|-------------|
| `AH-Thread-Id` | OpenCode `sessionID` | Groups requests from the same session |
| `AH-Trace-Id` | OpenCode `message.id` | Unique identifier for each message |

### Enable the Plugin

Add the plugin to your `opencode.json`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "plugin": ["opencode-llm-proxy-tracing"]
}
```

OpenCode will automatically install the plugin when needed.

### Custom Header Configuration (Optional)

By default, the plugin uses `AH-Thread-Id` and `AH-Trace-Id` header keys. You can override these with environment variables:

| Environment Variable | Default Value | Description |
|---------------------|---------------|-------------|
| `OPENCODE_LLM_PROXY_TRACING_THREAD_HEADER` | `AH-Thread-Id` | Custom thread header key |
| `OPENCODE_LLM_PROXY_TRACING_TRACE_HEADER` | `AH-Trace-Id` | Custom trace header key |

Example:

```bash
export OPENCODE_LLM_PROXY_TRACING_THREAD_HEADER="X-Thread-Id"
export OPENCODE_LLM_PROXY_TRACING_TRACE_HEADER="X-Trace-Id"
```

> **Note**: Empty string values will fall back to the default keys.

### Behavior Details

- **Thread ID**: Uses OpenCode's `sessionID` to group related requests
- **Trace ID**: Uses OpenCode's current user message `message.id` for unique identification
- If the current message has no `id`, only the thread header is injected

### Benefits

- **Session Aggregation**: Group related requests from the same OpenCode session in llm-proxy traces
- **Request Correlation**: Track individual messages across your AI infrastructure
- **Flexible Configuration**: Customize header keys to match your existing tracing infrastructure

---

## Troubleshooting

### OpenCode Cannot Connect

**Symptoms**: Connection errors, timeout errors

**Solutions**:
1. Verify `baseURL` points to the correct llm-proxy endpoint
2. Check that llm-proxy is running: `curl http://localhost:8090/health`
3. Verify firewall allows outbound connections
4. For HTTPS endpoints with self-signed certificates, configure trust settings

### Authentication Errors

**Symptoms**: 401 Unauthorized, 403 Forbidden

**Solutions**:
1. Verify your API key is correct in the configuration
2. Check that the API key has not expired in llm-proxy console
3. Ensure the API key has access to the requested project
4. Verify the API key has permissions for the requested models

### Unexpected Model Responses

**Symptoms**: Wrong model responding, unexpected behavior

**Solutions**:
1. Review active profile mappings in the llm-proxy console
2. Check channel configuration and model associations
3. Verify the requested model name matches your configuration
4. Disable or adjust profile rules if necessary

### Configuration Not Loading

**Symptoms**: OpenCode uses default settings, ignores config file

**Solutions**:
1. Verify config file location: `~/.config/opencode/opencode.json`
2. Check JSON syntax is valid (use a JSON validator)
3. Ensure file permissions allow reading
4. Restart OpenCode after configuration changes

---

## Advanced Configuration

### Multiple llm-proxy Providers

You can configure multiple llm-proxy instances as different providers:

```json
{
  "provider": {
    "llm-proxy-prod": {
      "npm": "@ai-sdk/anthropic",
      "name": "llm-proxy Production",
      "options": {
        "baseURL": "https://prod.llm-proxy.com/anthropic/v1",
        "apiKey": "prod-api-key"
      },
      "models": {
        "claude-sonnet-4-5": {
          "name": "Production - Claude Sonnet 4.5",
          "modalities": {
            "input": ["text", "image"],
            "output": ["text"]
          }
        }
      }
    },
    "llm-proxy-dev": {
      "npm": "@ai-sdk/anthropic",
      "name": "llm-proxy Development",
      "options": {
        "baseURL": "http://localhost:8090/anthropic/v1",
        "apiKey": "dev-api-key"
      },
      "models": {
        "claude-sonnet-4-5": {
          "name": "Development - Claude Sonnet 4.5",
          "modalities": {
            "input": ["text", "image"],
            "output": ["text"]
          }
        }
      }
    }
  }
}
```

### Using OpenAI-Compatible Endpoint

OpenCode can also use llm-proxy's OpenAI-compatible endpoint:

```json
{
  "provider": {
    "llm-proxy-openai": {
      "npm": "@ai-sdk/openai",
      "name": "llm-proxy OpenAI",
      "options": {
        "baseURL": "http://127.0.0.1:8090/v1",
        "apiKey": "your-llm-proxy-api-key"
      },
      "models": {
        "gpt-4": {
          "name": "llm-proxy - GPT-4",
          "modalities": {
            "input": ["text"],
            "output": ["text"]
          }
        }
      }
    }
  }
}
```

---

## Best Practices

### Security
- **Never commit API keys**: Use environment variables or secure vaults
- **Rotate keys regularly**: Update API keys periodically
- **Use HTTPS in production**: Always use encrypted connections for remote instances
- **Restrict API key permissions**: Grant only necessary permissions

### Performance
- **Enable trace aggregation**: Improves cache hit rates
- **Use appropriate models**: Match model capabilities to task complexity
- **Monitor usage**: Track costs and performance in llm-proxy console
- **Configure timeouts**: Set reasonable timeout values for your use case

---

## Related Documentation
- [Tracing Guide](tracing.md)
- [API Key Profiles Guide](api-key-profiles.md)
- [Model Management Guide](model-management.md)
- [Channel Management Guide](channel-management.md)
- [Anthropic API Reference](../api-reference/anthropic-api.md)
- README sections on [Usage Guide](../../../README.md#usage-guide)
