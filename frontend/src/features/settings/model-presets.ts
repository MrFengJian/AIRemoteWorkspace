/**
 * Presets for common OpenAI-compatible LLM providers. They only prefill the
 * add-provider form (name / base URL / a few model ids) — the live model list
 * can always be fetched via the provider's /models endpoint or edited by hand.
 */
export interface ProviderPreset {
  name: string;
  baseUrl: string;
  models: string[];
}

export const PROVIDER_PRESETS: ProviderPreset[] = [
  {
    name: "OpenAI",
    baseUrl: "https://api.openai.com/v1",
    models: ["gpt-4o", "gpt-4o-mini", "gpt-4.1", "o3-mini"],
  },
  {
    name: "Anthropic",
    baseUrl: "https://api.anthropic.com/v1",
    models: ["claude-sonnet-4-5", "claude-haiku-4-5", "claude-opus-4-1"],
  },
  {
    name: "DeepSeek",
    baseUrl: "https://api.deepseek.com/v1",
    models: ["deepseek-chat", "deepseek-reasoner"],
  },
  {
    name: "Moonshot Kimi",
    baseUrl: "https://api.moonshot.cn/v1",
    models: ["moonshot-v1-128k", "moonshot-v1-32k", "kimi-k2-0905-preview"],
  },
  {
    name: "Zhipu GLM",
    baseUrl: "https://open.bigmodel.cn/api/paas/v4",
    models: ["glm-4.6", "glm-4.5-air", "glm-4-flash"],
  },
  {
    name: "Alibaba Qwen",
    baseUrl: "https://dashscope.aliyuncs.com/compatible-mode/v1",
    models: ["qwen3-max", "qwen-max", "qwen-plus", "qwen-turbo"],
  },
  {
    name: "SiliconFlow",
    baseUrl: "https://api.siliconflow.cn/v1",
    models: [
      "deepseek-ai/DeepSeek-V3",
      "Qwen/Qwen2.5-72B-Instruct",
      "THUDM/glm-4-9b-chat",
    ],
  },
  {
    name: "OpenRouter",
    baseUrl: "https://openrouter.ai/api/v1",
    models: ["openai/gpt-4o-mini", "anthropic/claude-sonnet-4.5", "deepseek/deepseek-chat"],
  },
  {
    name: "Ollama (local)",
    baseUrl: "http://localhost:11434/v1",
    models: ["qwen3", "llama3.1"],
  },
];
