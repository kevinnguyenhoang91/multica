# OpenClaw Channel 系统源码调研

> 源码位置: `~/Desktop/参考项目/openclaw`
>
> 调研目的: 深入理解 OpenClaw 的 Channel 架构、消息流转机制、第三方渠道集成模式，为 Super Multica 的 Channel 系统设计提供参考。

---

## 目录

1. [项目整体结构](#1-项目整体结构)
2. [Channel 插件体系架构](#2-channel-插件体系架构)
3. [核心类型定义](#3-核心类型定义)
4. [插件发现与加载机制](#4-插件发现与加载机制)
5. [路由与会话管理](#5-路由与会话管理)
6. [消息流转: 正常发消息 → AI 回复 全链路](#6-消息流转-正常发消息--ai-回复-全链路)
7. [第三方渠道集成: Telegram 完整流程](#7-第三方渠道集成-telegram-完整流程)
8. [Outbound 投递管线](#8-outbound-投递管线)
9. [安全与访问控制](#9-安全与访问控制)
10. [Channel Manager 生命周期管理](#10-channel-manager-生命周期管理)
11. [设计亮点与可借鉴之处](#11-设计亮点与可借鉴之处)
12. [关键文件索引](#12-关键文件索引)

---

## 1. 项目整体结构

```
openclaw/
├── src/                          # 核心模块
│   ├── channels/                 # Channel 插件系统 (类型、注册表、工具函数)
│   │   ├── registry.ts           # 内置 Channel 元信息注册表
│   │   ├── plugins/              # 插件类型定义与加载
│   │   │   ├── types.core.ts     # 基础类型 (ChannelId, ChannelMeta, ChannelCapabilities)
│   │   │   ├── types.adapters.ts # Adapter 接口 (Config, Outbound, Gateway, Security...)
│   │   │   ├── types.plugin.ts   # ChannelPlugin 顶层接口
│   │   │   ├── catalog.ts        # 插件发现与目录管理
│   │   │   └── load.ts           # 插件加载 (带缓存)
│   │   ├── mention-gating.ts     # 群组 @提及 门控逻辑
│   │   ├── sender-identity.ts    # 发送者身份验证
│   │   ├── chat-type.ts          # 聊天类型标准化 (direct/group/channel/thread)
│   │   └── ack-reactions.ts      # ACK 表情反应
│   ├── telegram/                 # Telegram 内置实现
│   │   ├── monitor.ts            # 长轮询/Webhook 启动入口
│   │   ├── webhook.ts            # HTTP Webhook 服务器
│   │   ├── bot.ts                # Grammy Bot 创建与中间件编排
│   │   ├── bot-handlers.ts       # 消息/回调/反应处理器注册
│   │   ├── bot-message.ts        # 消息处理器工厂
│   │   ├── bot-message-context.ts # Inbound 上下文构建 (路由、安全、信封)
│   │   ├── bot-message-dispatch.ts # 调度到 Agent 并处理流式回复
│   │   ├── bot/delivery.ts       # 回复投递 (文本分块、媒体、线程)
│   │   └── send.ts               # 独立 Outbound 发送函数
│   ├── routing/                  # 消息路由与会话管理
│   │   ├── resolve-route.ts      # Agent 路由解析 (binding 匹配)
│   │   ├── bindings.ts           # 路由绑定配置读取
│   │   └── session-key.ts        # 会话 Key 构建 (DM/Group/Thread)
│   ├── plugins/                  # 通用插件系统
│   │   ├── registry.ts           # 插件注册表 (工具/钩子/Channel/Provider)
│   │   ├── runtime.ts            # 全局插件注册表单例 (Symbol-based)
│   │   ├── loader.ts             # 插件加载器 (jiti + discovery)
│   │   └── discovery.ts          # 插件发现
│   ├── infra/outbound/           # Outbound 投递基础设施
│   │   ├── deliver.ts            # 主投递编排
│   │   ├── payloads.ts           # Payload 标准化
│   │   ├── channel-selection.ts  # 多 Channel 选择
│   │   └── target-resolver.ts    # 目标解析 (带缓存)
│   ├── auto-reply/               # Agent 回复管线
│   │   ├── dispatch.ts           # 入站消息调度
│   │   ├── reply/                # 回复生成
│   │   │   ├── dispatch-from-config.ts  # 核心回复流程
│   │   │   └── get-reply.ts      # LLM 调用
│   │   ├── types.ts              # ReplyPayload, GetReplyOptions
│   │   └── envelope.ts           # 消息信封格式化
│   ├── gateway/                  # WebSocket 网关
│   │   └── server-channels.ts    # ChannelManager 生命周期管理
│   └── config/                   # 配置类型
│       ├── types.channels.ts     # Channel 配置汇总
│       └── types.telegram.ts     # Telegram 专属配置
├── extensions/                   # 33+ 外部插件
│   ├── telegram/                 # Telegram Channel 插件
│   │   ├── index.ts              # 插件入口 (register)
│   │   └── src/
│   │       ├── channel.ts        # ChannelPlugin 完整实现
│   │       └── runtime.ts        # 全局 Runtime
│   ├── discord/                  # Discord Channel 插件
│   ├── slack/                    # Slack Channel 插件
│   ├── signal/                   # Signal Channel 插件
│   └── ...                       # 更多渠道
└── apps/                         # Web/Desktop 应用
```

---

## 2. Channel 插件体系架构

OpenClaw 采用**插件化 Adapter 模式**来统一所有 Channel 的接入。每个 Channel 实现一个 `ChannelPlugin` 合约，包含多个可选的 Adapter:

```
┌─────────────────────────────────────────────────────────────┐
│                     ChannelPlugin                            │
├─────────┬──────────┬──────────┬──────────┬─────────────────┤
│  config  │ outbound │ gateway  │ security │    其他 Adapter   │
│          │          │          │          │                   │
│ 账号管理  │ 消息发送  │ 生命周期  │ 访问控制  │ groups, mentions  │
│ 启用检查  │ 媒体发送  │ start    │ DM策略   │ directory, status │
│ 配置描述  │ 目标解析  │ stop     │ 告警收集  │ actions, threading│
│          │ 文本分块  │ QR登录   │          │ heartbeat, setup  │
└─────────┴──────────┴──────────┴──────────┴─────────────────┘
```

### ChannelPlugin 接口

```typescript
// src/channels/plugins/types.plugin.ts
type ChannelPlugin<ResolvedAccount = any, Probe = unknown, Audit = unknown> = {
  id: ChannelId;                         // "telegram" | "discord" | ...
  meta: ChannelMeta;                     // 元信息 (标签、图标、文档路径)
  capabilities: ChannelCapabilities;     // 能力声明 (chatTypes, reactions, threads...)

  // --- 必选 Adapter ---
  config: ChannelConfigAdapter<ResolvedAccount>;  // 账号配置管理

  // --- 可选 Adapter ---
  outbound?: ChannelOutboundAdapter;     // 消息发送
  gateway?: ChannelGatewayAdapter;       // 生命周期 (启动/停止/QR登录)
  security?: ChannelSecurityAdapter;     // DM安全策略
  setup?: ChannelSetupAdapter;           // 初始化配置
  groups?: ChannelGroupAdapter;          // 群组行为
  mentions?: ChannelMentionAdapter;      // @提及处理
  status?: ChannelStatusAdapter;         // 状态监控
  directory?: ChannelDirectoryAdapter;   // 联系人/群组目录
  actions?: ChannelMessageActionAdapter; // 消息动作 (反应、按钮、卡片)
  threading?: ChannelThreadingAdapter;   // 线程处理
  streaming?: ChannelStreamingAdapter;   // 流式输出
  messaging?: ChannelMessagingAdapter;   // 目标格式化
  auth?: ChannelAuthAdapter;             // 认证
  heartbeat?: ChannelHeartbeatAdapter;   // 心跳检测
  pairing?: ChannelPairingAdapter;       // 配对/白名单
  elevated?: ChannelElevatedAdapter;     // 提权
  commands?: ChannelCommandAdapter;      // 命令控制
  agentPrompt?: ChannelAgentPromptAdapter; // Agent 提示词
  resolver?: ChannelResolverAdapter;     // 目标解析
  agentTools?: ChannelAgentToolFactory;  // Channel 自带的 Agent 工具
};
```

### Adapter 职责一览

| Adapter | 职责 | 关键方法 |
|---------|------|---------|
| **config** | 账号管理 | `listAccountIds`, `resolveAccount`, `isConfigured`, `isEnabled` |
| **outbound** | 消息发送 | `sendText`, `sendMedia`, `sendPayload`, `resolveTarget` |
| **gateway** | 生命周期 | `startAccount`, `stopAccount`, `loginWithQrStart`, `loginWithQrWait` |
| **security** | 访问控制 | `resolveDmPolicy`, `collectWarnings` |
| **groups** | 群组行为 | `resolveRequireMention`, `resolveToolPolicy` |
| **status** | 状态监控 | `probeAccount`, `auditAccount`, `buildAccountSnapshot` |
| **directory** | 目录查询 | `listPeers`, `listGroups`, `listGroupMembers` |
| **actions** | 消息交互 | `handleAction` (reactions, buttons, cards, polls) |
| **mentions** | @提及 | `stripMentions`, `stripPatterns` |
| **setup** | 初始化 | `applyAccountConfig`, `validateInput` |
| **pairing** | 配对 | `normalizeAllowEntry`, `notifyApproval` |

---

## 3. 核心类型定义

### ChannelCapabilities — 渠道能力声明

```typescript
// src/channels/plugins/types.core.ts
type ChannelCapabilities = {
  chatTypes: Array<"direct" | "group" | "channel" | "thread">;
  polls?: boolean;       // 原生投票
  reactions?: boolean;   // 表情反应
  edit?: boolean;        // 编辑消息
  unsend?: boolean;      // 撤回消息
  reply?: boolean;       // 引用回复
  threads?: boolean;     // 线程支持
  media?: boolean;       // 媒体支持
  nativeCommands?: boolean;  // 原生命令 (如 Telegram /start)
  blockStreaming?: boolean;  // 流式输出聚合
};
```

### ChannelMeta — 渠道元信息

```typescript
type ChannelMeta = {
  id: ChannelId;
  label: string;              // "Telegram"
  selectionLabel: string;     // "Telegram (Bot API)"
  detailLabel?: string;       // "Telegram Bot"
  docsPath: string;           // "/channels/telegram"
  blurb: string;              // 简介
  systemImage?: string;       // SF Symbol 图标名
  aliases?: string[];         // 别名
  order?: number;             // 排序权重
  // ...
};
```

### ChannelAccountSnapshot — 账号运行时快照

```typescript
type ChannelAccountSnapshot = {
  accountId: string;
  name?: string;
  enabled?: boolean;
  configured?: boolean;
  running?: boolean;
  connected?: boolean;
  lastConnectedAt?: number | null;
  lastMessageAt?: number | null;
  lastError?: string | null;
  lastStartAt?: number | null;
  lastStopAt?: number | null;
  dmPolicy?: string;
  allowFrom?: string[];
  // ...
};
```

### ReplyPayload — Agent 回复载荷

```typescript
// src/auto-reply/types.ts
type ReplyPayload = {
  text?: string;
  mediaUrl?: string;
  mediaUrls?: string[];
  replyToId?: string;
  audioAsVoice?: boolean;
  isError?: boolean;
  channelData?: Record<string, unknown>;
};
```

---

## 4. 插件发现与加载机制

### 4.1 插件发现

```
发现源 (优先级从高到低):
  1. config — 配置文件指定的路径 (plugins.load.paths)
  2. workspace — 项目本地 extensions/
  3. global — ~/.super-multica/extensions/
  4. bundled — 内置 extensions/
```

每个插件目录需包含 `openclaw.plugin.json` 清单文件，声明插件 ID、名称、类型、配置 Schema 等。

### 4.2 插件加载流程

```
loadOpenClawPlugins(options)
  │
  ├─ normalizePluginsConfig()      // 处理 allow/deny 列表
  ├─ 检查缓存 (cacheKey = workspace + plugins config)
  │
  ├─ discoverOpenClawPlugins()     // 扫描插件候选
  ├─ loadPluginManifestRegistry()  // 加载清单文件
  │
  ├─ for each candidate:
  │   ├─ 检查启用/禁用状态
  │   ├─ 验证配置 Schema (JSON Schema)
  │   ├─ jiti(candidate.source)    // 使用 jiti 动态加载 TypeScript
  │   ├─ 解析 module export (default export or register/activate)
  │   ├─ createApi(record, config) // 创建插件 API
  │   └─ register(api)             // 调用插件注册函数
  │       ├─ api.registerChannel({ plugin: channelPlugin })
  │       ├─ api.registerTool(tool)
  │       ├─ api.registerHook(events, handler)
  │       └─ api.registerProvider(provider)
  │
  ├─ setActivePluginRegistry(registry)  // 设置为全局活跃注册表
  └─ return registry
```

### 4.3 插件注册表 (全局单例)

```typescript
// src/plugins/runtime.ts
const REGISTRY_STATE = Symbol.for("openclaw.pluginRegistryState");

// 使用 Symbol 确保跨模块共享同一个注册表实例
type RegistryState = {
  registry: PluginRegistry | null;
  key: string | null;
};

export function setActivePluginRegistry(registry: PluginRegistry, cacheKey?: string);
export function getActivePluginRegistry(): PluginRegistry | null;
export function requireActivePluginRegistry(): PluginRegistry;
```

### 4.4 Telegram 插件注册示例

```typescript
// extensions/telegram/index.ts
const plugin = {
  id: "telegram",
  name: "Telegram",
  register(api: OpenClawPluginApi) {
    setTelegramRuntime(api.runtime);           // 保存全局 Runtime
    api.registerChannel({ plugin: telegramPlugin }); // 注册 Channel 插件
  },
};
export default plugin;
```

---

## 5. 路由与会话管理

### 5.1 路由解析 (Binding 匹配)

当一条消息进入系统时，需要确定由哪个 Agent 处理。OpenClaw 使用 **Binding 优先级匹配**:

```typescript
// src/routing/resolve-route.ts
function resolveAgentRoute(input: ResolveAgentRouteInput): ResolvedAgentRoute {
  // input: { cfg, channel, accountId, peer, parentPeer, guildId, teamId }

  // 1. 过滤出匹配 channel + accountId 的 bindings
  const bindings = listBindings(cfg).filter(b =>
    matchesChannel(b.match, channel) && matchesAccountId(b.match?.accountId, accountId)
  );

  // 2. 按优先级匹配
  //    peer (DM/群组精确匹配) → parentPeer (线程父级继承)
  //    → guild (Discord服务器) → team (MS Teams团队)
  //    → account (账号级别) → channel (渠道级别) → default (默认Agent)

  // 3. 返回结果
  return {
    agentId: "assistant",
    channel: "telegram",
    accountId: "default",
    sessionKey: "agent:assistant:peer:telegram:default:dm:123456",
    mainSessionKey: "agent:assistant:main",
    matchedBy: "binding.peer",  // 调试信息
  };
}
```

### 5.2 Binding 配置

```typescript
// 配置文件中的 bindings 数组
type AgentBinding = {
  agentId: string;           // 目标 Agent ID
  match?: {
    channel?: string;        // "telegram"
    accountId?: string;      // "default" 或 "*" (匹配所有)
    peer?: { kind: string; id: string };  // 精确匹配特定聊天
    guildId?: string;        // Discord 服务器
    teamId?: string;         // MS Teams 团队
  };
};
```

### 5.3 Session Key 构建

Session Key 是会话持久化的核心标识，格式根据 DM Scope 不同而变化:

```
DM Scope 模式:
  "main"                    → agent:{agentId}:main
  "per-peer"                → agent:{agentId}:dm:{peerId}
  "per-channel-peer"        → agent:{agentId}:{channel}:dm:{peerId}
  "per-account-channel-peer"→ agent:{agentId}:{channel}:{accountId}:dm:{peerId}

Group/Channel:
  → agent:{agentId}:{channel}:{peerKind}:{peerId}

Thread (线程):
  → {baseSessionKey}:thread:{threadId}
```

**Identity Linking**: 支持跨渠道身份关联，例如 Telegram 用户 `123` 和 WhatsApp 用户 `456` 映射到同一个 canonical ID，共享同一个 session。

```typescript
// 配置
session: {
  dmScope: "per-peer",
  identityLinks: {
    "alice": ["telegram:123", "whatsapp:456"],
  }
}
```

---

## 6. 消息流转: 正常发消息 → AI 回复 全链路

以下是一条用户消息从进入系统到 AI 回复的**完整流转路径**:

```
                           ┌─────────────────────────────────┐
                           │           用户发送消息            │
                           └──────────────┬──────────────────┘
                                          │
                           ┌──────────────▼──────────────────┐
                           │     Channel 接收 (Inbound)       │
                           │  Polling / Webhook / WebSocket   │
                           └──────────────┬──────────────────┘
                                          │
                    ┌─────────────────────▼─────────────────────┐
                    │            消息预处理 (bot-handlers)         │
                    │                                             │
                    │  • 去重 (update offset + dedup)             │
                    │  • 媒体组缓冲 (multi-image → single event)  │
                    │  • 文本片段组装 (>4000 字符分片重组)          │
                    │  • Inbound 防抖 (快速连续消息合并)            │
                    │  • 媒体文件下载 (图片/视频/语音/贴纸)        │
                    └─────────────────────┬─────────────────────┘
                                          │
                    ┌─────────────────────▼─────────────────────┐
                    │          上下文构建 (bot-message-context)    │
                    │                                             │
                    │  1. 解析 chatType (DM / Group / Thread)     │
                    │  2. 解析 Agent 路由 (resolveAgentRoute)      │
                    │     → agentId, sessionKey                   │
                    │  3. 安全检查:                                │
                    │     - DM 策略 (pairing/allowlist/open)      │
                    │     - Group 策略 (open/allowlist/disabled)   │
                    │  4. @提及检测与门控                           │
                    │     - 显式 @bot 提及                         │
                    │     - 正则模式匹配                           │
                    │     - 回复链隐式提及                         │
                    │  5. 发送 ACK 表情 (👀 处理中)               │
                    │  6. 构建消息信封 [Channel From Time] body     │
                    │  7. 提取上下文 (引用/转发/位置/群历史)        │
                    └─────────────────────┬─────────────────────┘
                                          │
                    ┌─────────────────────▼─────────────────────┐
                    │       调度到 Agent (bot-message-dispatch)    │
                    │                                             │
                    │  1. 设置流式输出模式:                        │
                    │     - "off": 等完整回复再发送                 │
                    │     - "partial": 逐 token 实时编辑消息        │
                    │     - "block": 语义块级流式                  │
                    │  2. 调用 dispatchReplyFromConfig()            │
                    └─────────────────────┬─────────────────────┘
                                          │
              ┌───────────────────────────▼───────────────────────────┐
              │             Auto-Reply 管线 (dispatch-from-config)      │
              │                                                         │
              │  1. 检查重复入站消息                                      │
              │  2. 触发 message_received 钩子 (插件)                    │
              │  3. 检查 /stop 命令 (快速中断)                           │
              │  4. 调用 getReplyFromConfig() → LLM 推理                │
              │     ├─ 加载 session transcript                          │
              │     ├─ 构建 Agent 上下文 (system prompt + tools + skills)│
              │     ├─ 调用 LLM (OpenAI/Anthropic/DeepSeek/...)         │
              │     ├─ 执行 tools (如需要)                              │
              │     └─ 生成 ReplyPayload                                │
              │  5. 应用 TTS (如配置)                                    │
              │  6. 处理跨 Channel 回复路由                               │
              └───────────────────────────┬───────────────────────────┘
                                          │
                    ┌─────────────────────▼─────────────────────┐
                    │        回复投递 (delivery / deliver.ts)      │
                    │                                             │
                    │  1. 加载 Channel Outbound Adapter            │
                    │  2. 标准化 Payload (解析指令, 合并媒体)       │
                    │  3. 文本分块:                                │
                    │     - 按字符限制 (Telegram: 4096)            │
                    │     - 按段落/Markdown 块                     │
                    │     - Signal: Markdown → 富文本样式           │
                    │  4. 发送:                                    │
                    │     - sendText(text)                         │
                    │     - sendMedia(caption, mediaUrl)           │
                    │     - sendPayload(payload) (channelData)     │
                    │  5. 线程引用 (replyToId / threadId)          │
                    │  6. 移除 ACK 表情                            │
                    │  7. 记录 session transcript                  │
                    └─────────────────────┬─────────────────────┘
                                          │
                           ┌──────────────▼──────────────────┐
                           │     Channel 发送 (Outbound)      │
                           │    Channel API → 用户收到回复     │
                           └──────────────────────────────────┘
```

### 关键步骤详解

#### Step 1: 消息接收

Channel 通过两种方式接收消息:
- **Long Polling**: 主动轮询 API 获取新消息 (Telegram, WhatsApp)
- **Webhook**: 被动接收 HTTP POST 推送 (Telegram 可选, Google Chat)
- **WebSocket**: 实时双向连接 (Discord via discord.js, Slack Socket Mode)

#### Step 2: 消息预处理

```typescript
// Telegram 特有的预处理:

// 1. 媒体组缓冲 — Telegram 将多图消息拆成多个 update，需要合并
const MEDIA_GROUP_TIMEOUT_MS = 1500; // 等待 1.5s 收集同组媒体

// 2. 文本片段重组 — 超长消息被 Telegram 分片
const TEXT_FRAGMENT_START_THRESHOLD = 4000; // >4000字符触发分片检测

// 3. Inbound 防抖 — 用户快速连发消息时合并处理
createInboundDebouncer({ delayMs, maxWaitMs });
```

#### Step 3: 路由解析

```typescript
const route = resolveAgentRoute({
  cfg,
  channel: "telegram",
  accountId: "default",
  peer: { kind: "dm", id: "123456" },
});
// → { agentId: "assistant", sessionKey: "agent:assistant:main", matchedBy: "default" }
```

#### Step 4: Agent 调用

核心函数 `getReplyFromConfig()` 负责:
1. 从 sessionKey 加载历史 transcript
2. 根据 agentId 加载 Agent 配置 (system prompt, tools, skills)
3. 调用 LLM Provider (支持 OpenAI, Anthropic, DeepSeek, Kimi, Groq, Mistral, Google, Together)
4. 处理 tool calls (循环执行)
5. 返回 `ReplyPayload[]`

#### Step 5: 回复投递

```typescript
await deliverOutboundPayloads({
  cfg,
  channel: "telegram",
  to: "123456",
  accountId: "default",
  payloads: [{ text: "Hello! I'm your AI assistant.", mediaUrl: "..." }],
  replyToId: originalMessageId,
});
```

---

## 7. 第三方渠道集成: Telegram 完整流程

以 Telegram 为例，详细说明第三方渠道的集成方式和消息流转。

### 7.1 插件注册

```typescript
// extensions/telegram/index.ts
export default {
  id: "telegram",
  register(api: OpenClawPluginApi) {
    setTelegramRuntime(api.runtime);
    api.registerChannel({ plugin: telegramPlugin });
  },
};
```

### 7.2 Channel Plugin 实现

```typescript
// extensions/telegram/src/channel.ts
export const telegramPlugin: ChannelPlugin<ResolvedTelegramAccount, TelegramProbe> = {
  id: "telegram",
  meta: getChatChannelMeta("telegram"),
  capabilities: {
    chatTypes: ["direct", "group", "channel", "thread"],
    reactions: true,
    threads: true,
    media: true,
    nativeCommands: true,
    blockStreaming: true,
  },

  config: {
    listAccountIds(cfg) {
      // 返回配置中的所有 Telegram 账号 ID
      return Object.keys(cfg.channels?.telegram?.accounts ?? {});
    },
    resolveAccount(cfg, accountId) {
      // 解析账号配置 (botToken, dmPolicy, allowFrom 等)
    },
    isConfigured(account) {
      // 检查 botToken 是否存在
    },
    isEnabled(account) {
      return account.enabled !== false;
    },
  },

  outbound: {
    deliveryMode: "direct",      // 直接调用 Bot API
    textChunkLimit: 4000,        // Telegram 限制
    chunker: markdownToTelegramChunks, // Markdown → Telegram HTML 分块

    async sendText(ctx) {
      return sendMessageTelegram(ctx.to, ctx.text, {
        accountId: ctx.accountId,
        replyToId: ctx.replyToId,
        threadId: ctx.threadId,
      });
    },

    async sendMedia(ctx) {
      return sendMessageTelegram(ctx.to, ctx.text, {
        mediaUrl: ctx.mediaUrl,
        accountId: ctx.accountId,
      });
    },

    resolveTarget({ to, allowFrom, accountId }) {
      // 验证并标准化 Telegram chat ID
      // 支持: 纯数字 ID, @username, t.me/ 链接
    },
  },

  gateway: {
    async startAccount(ctx) {
      // 启动 Telegram 监听
      return monitorTelegramProvider({
        cfg: ctx.cfg,
        accountId: ctx.accountId,
        abortSignal: ctx.abortSignal,
        setStatus: ctx.setStatus,
      });
    },

    async stopAccount(ctx) {
      // 通过 AbortController 停止
      ctx.abortSignal.abort();
    },
  },

  security: {
    resolveDmPolicy(ctx) {
      return {
        policy: ctx.account.dmPolicy ?? "pairing",
        allowFrom: ctx.account.allowFrom,
        approveHint: "approve via /allow command",
      };
    },
  },
};
```

### 7.3 Telegram 消息接收 (Inbound) 详细流程

```
Telegram 用户发送消息
        │
        ▼
┌───────────────────────────────────────┐
│  Telegram API Server                  │
│  (api.telegram.org)                   │
└───────────────┬───────────────────────┘
                │
    ┌───────────┴───────────┐
    │                       │
    ▼                       ▼
┌─────────┐          ┌──────────┐
│ Polling  │          │ Webhook  │
│ (默认)   │          │ (可选)    │
│          │          │          │
│ Grammy   │          │ HTTP POST│
│ Runner   │          │ /webhook │
│ getUpdates│         │ grammy   │
│ + backoff│          │ callback │
└────┬─────┘          └────┬─────┘
     │                     │
     └──────────┬──────────┘
                │
                ▼
┌──────────────────────────────────────┐
│  Grammy Middleware Pipeline           │
│                                      │
│  1. apiThrottler() — 速率限制        │
│  2. sequentialize() — 按 chat/topic  │
│     序列化更新, 保证处理顺序          │
│  3. 原始更新日志 (debug)             │
│  4. Update offset 追踪 + 去重        │
└───────────────┬──────────────────────┘
                │
                ▼
┌──────────────────────────────────────┐
│  bot.on("message") Handler           │
│  (bot-handlers.ts — 928 行)          │
│                                      │
│  1. 验证 chatType, 群组策略          │
│  2. 文本片段缓冲 (>4000字符)         │
│  3. 媒体组缓冲 (多图合并)            │
│  4. 单媒体解析 (resolveMedia)        │
│  5. Inbound 防抖 (快速连发合并)      │
│  6. 调用 processMessage()            │
└───────────────┬──────────────────────┘
                │
                ▼
┌──────────────────────────────────────┐
│  processMessage()                     │
│  (bot-message.ts)                    │
│                                      │
│  ┌─ buildTelegramMessageContext()    │
│  │  (bot-message-context.ts 700行)   │
│  │                                   │
│  │  • 记录 channel activity          │
│  │  • 解析 chatType + threadId       │
│  │  • resolveAgentRoute() → agent    │
│  │  • DM 安全检查:                   │
│  │    - "pairing": 发送配对码        │
│  │    - "allowlist": 检查白名单      │
│  │    - "open": 放行                 │
│  │  • Group 安全检查                 │
│  │  • @提及检测:                     │
│  │    - @bot 显式提及                │
│  │    - 正则模式匹配                 │
│  │    - 回复链隐式提及               │
│  │  • Mention Gating (群组中未提及   │
│  │    则跳过)                        │
│  │  • 发送 ACK 反应 (👀)            │
│  │  • formatInboundEnvelope()        │
│  │  • 提取引用/转发/位置/贴纸        │
│  │  • 群组历史上下文                 │
│  └────────────────────────┐          │
│                           │          │
│  ┌─ dispatchTelegramMessage()        │
│  │  (bot-message-dispatch.ts 357行)  │
│  │                                   │
│  │  • 配置流式模式:                  │
│  │    - "off": 完整回复后发送        │
│  │    - "partial": token级实时编辑   │
│  │    - "block": 语义块级流式        │
│  │  • dispatchReplyFromConfig()      │
│  │    → getReplyFromConfig()         │
│  │    → LLM 推理 + Tool 执行        │
│  │  • 流式回调:                      │
│  │    - onBlockReply → 编辑草稿消息  │
│  │    - onToolResult → 中间结果      │
│  │  • deliverReplies() → 发送回复    │
│  │  • 移除 ACK 反应                  │
│  └───────────────────────────────────┘
└──────────────────────────────────────┘
```

### 7.4 Telegram 消息发送 (Outbound) 详细流程

```typescript
// src/telegram/send.ts — 754 行

async function sendMessageTelegram(
  to: string,
  text: string,
  opts?: {
    mediaUrl?: string;
    accountId?: string;
    replyToId?: string;
    threadId?: string | number;
    retry?: OutboundRetryConfig;
  }
): Promise<OutboundDeliveryResult> {

  // 1. 解析账号配置、Bot Token、代理
  const account = resolveAccount(cfg, opts.accountId);
  const token = account.botToken;
  const api = new Api(token, { proxy });

  // 2. 标准化 chatId
  //    支持: "123456", "@channel_name", "t.me/+xxxxx"
  const chatId = normalizeTelegramChatId(to);

  // 3. 文本转换: Markdown → Telegram HTML
  const html = markdownToTelegramHtml(text);

  // 4. 发送消息
  if (opts.mediaUrl) {
    // 带媒体: sendPhoto / sendVideo / sendAudio / sendVoice / sendDocument / sendAnimation
    const mediaType = detectMediaType(opts.mediaUrl);
    const result = await api[`send${mediaType}`](chatId, {
      caption: html,
      parse_mode: "HTML",
      reply_parameters: opts.replyToId ? { message_id: opts.replyToId } : undefined,
      message_thread_id: opts.threadId,
    });
    return { channel: "telegram", messageId: result.message_id, chatId };
  } else {
    // 纯文本
    const result = await api.sendMessage(chatId, html, {
      parse_mode: "HTML",
      reply_parameters: opts.replyToId ? { message_id: opts.replyToId } : undefined,
      message_thread_id: opts.threadId,
      link_preview_options: { is_disabled: !account.linkPreview },
    });
    return { channel: "telegram", messageId: result.message_id, chatId };
  }

  // 5. 错误处理
  //    - HTML 解析失败 → 降级为纯文本重试
  //    - 网络错误 → 指数退避重试
  //    - 语音消息被禁止 → 降级为文档发送

  // 6. 记录已发送消息 (用于反应追踪)
}
```

### 7.5 Telegram 配置示例

```json5
// ~/.super-multica/credentials.json5
{
  channels: {
    telegram: {
      accounts: {
        default: {
          botToken: "123456:ABC-DEF...",  // BotFather 获取
          // 或 tokenFile: "/path/to/token",  // 密钥管理器
          dmPolicy: "pairing",           // DM 安全策略
          allowFrom: [123456789],        // 白名单 (Telegram user ID)
          groupPolicy: "open",           // 群组策略
          streamMode: "partial",         // 流式输出模式
          textChunkLimit: 4000,          // 文本分块大小
          replyToMode: "first",          // 引用回复模式
          reactionLevel: "ack",          // ACK 反应级别
          actions: {
            reactions: true,
            sendMessage: true,
          },
          groups: {
            "-1001234567890": {          // 群组 ID
              requireMention: true,       // 需要 @提及
              tools: { allow: ["search", "calculator"] },
              topics: {
                "42": { enabled: true },  // 论坛 topic
              },
            },
          },
        },
      },
    },
  },
}
```

---

## 8. Outbound 投递管线

### 8.1 投递编排

```typescript
// src/infra/outbound/deliver.ts

async function deliverOutboundPayloads(params: {
  cfg: OpenClawConfig;
  channel: "telegram" | "discord" | "slack" | ...;
  to: string;               // 目标 ID
  accountId?: string;
  payloads: ReplyPayload[];  // 回复载荷数组
  replyToId?: string;        // 引用的消息 ID
  threadId?: string | number;// 线程 ID
  abortSignal?: AbortSignal; // 中止信号
  mirror?: {                 // Session transcript 镜像
    sessionKey: string;
    text?: string;
  };
}): Promise<OutboundDeliveryResult[]> {

  // 1. 加载 Channel Outbound Adapter
  const handler = await createChannelHandler({
    cfg, channel, to, accountId, ...
  });
  //    → loadChannelOutboundAdapter(channel)
  //    → plugin.outbound.sendText / sendMedia

  // 2. 标准化 Payload
  const normalized = normalizeReplyPayloadsForDelivery(payloads);
  //    → 解析文本指令 (mediaUrl, replyToId)
  //    → 合并多个媒体 URL
  //    → 过滤空/静默 payload

  // 3. 逐个 Payload 发送
  for (const payload of normalized) {
    if (payload.mediaUrls.length === 0) {
      // 纯文本 → 分块发送
      await sendTextChunks(payload.text);
    } else {
      // 带媒体 → 逐媒体发送 (首个附带 caption)
      for (const url of payload.mediaUrls) {
        await handler.sendMedia(first ? payload.text : "", url);
      }
    }
  }

  // 4. 镜像到 session transcript
  if (params.mirror) {
    await appendAssistantMessageToSessionTranscript(mirror);
  }
}
```

### 8.2 文本分块策略

```
分块模式:
  "length" (默认) — 按字符限制硬切 (chunker 函数)
  "newline"      — 先按段落/换行拆分, 再按字符限制

特殊处理:
  Signal  — Markdown → 富文本样式 (SignalTextStyleRange)
  Telegram — Markdown → HTML (Telegram flavor)
  Discord  — 原生 Markdown + Embed
```

### 8.3 Channel 选择

当系统需要主动发消息（非回复），需要确定使用哪个 Channel:

```typescript
// src/infra/outbound/channel-selection.ts

async function resolveMessageChannelSelection(params: {
  cfg: OpenClawConfig;
  channel?: string;
}) {
  // 1. 如果指定了 channel, 直接使用
  // 2. 列出所有已配置的 channel
  // 3. 只有一个 → 自动选择
  // 4. 多个 → 抛错要求明确指定
}

async function listConfiguredMessageChannels(cfg) {
  // 遍历所有已注册的 channel 插件
  // 检查每个插件是否有启用且已配置的账号
  for (const plugin of listChannelPlugins()) {
    if (await isPluginConfigured(plugin, cfg)) {
      channels.push(plugin.id);
    }
  }
}
```

---

## 9. 安全与访问控制

### 9.1 DM 安全策略

```
策略类型 (dmPolicy):
  "pairing"  (默认) — 未知发送者收到配对码, 需管理员批准
  "allowlist"       — 仅允许 allowFrom 列表中的用户
  "open"            — 允许所有 DM (需 allowFrom 包含 "*")
  "disabled"        — 忽略所有 DM
```

**配对流程 (Pairing)**:
1. 未知用户发送 DM
2. 系统生成配对码，回复给用户
3. 管理员通过 `/allow` 命令批准
4. 用户 ID 被加入持久化白名单

### 9.2 群组安全策略

```
策略类型 (groupPolicy):
  "open"      — 绕过 allowFrom, 仅受 mention-gating 控制
  "allowlist"  — 仅允许 groupAllowFrom/allowFrom 中的发送者
  "disabled"   — 完全阻止群消息
```

### 9.3 Mention Gating (提及门控)

```typescript
// src/channels/mention-gating.ts

function resolveMentionGating(params: {
  requireMention: boolean;  // 是否需要 @提及
  canDetectMention: boolean; // 渠道是否能检测提及
  wasMentioned: boolean;     // 是否被提及
  implicitMention?: boolean; // 隐式提及 (回复链)
  shouldBypassMention?: boolean; // 命令绕过
}): {
  effectiveWasMentioned: boolean;
  shouldSkip: boolean;       // true = 跳过处理
};
```

在群组中，当 `requireMention = true` 时:
- 显式 `@bot` 提及 → 处理
- 回复 bot 消息 (隐式提及) → 处理
- 授权用户发送控制命令 → 绕过门控
- 其他消息 → 跳过

---

## 10. Channel Manager 生命周期管理

```typescript
// src/gateway/server-channels.ts

type ChannelManager = {
  getRuntimeSnapshot: () => ChannelRuntimeSnapshot; // 获取所有 channel 运行状态
  startChannels: () => Promise<void>;               // 启动所有已配置 channel
  startChannel: (channel, accountId?) => Promise<void>;  // 启动单个 channel
  stopChannel: (channel, accountId?) => Promise<void>;   // 停止单个 channel
  markChannelLoggedOut: (channelId, cleared, accountId?) => void; // 标记登出
};
```

### 启动流程

```
createChannelManager(opts)
  │
  ├─ startChannels()
  │   └─ for each plugin in listChannelPlugins():
  │       └─ startChannel(plugin.id)
  │
  └─ startChannel(channelId, accountId?)
      │
      ├─ 获取 plugin = getChannelPlugin(channelId)
      ├─ 获取 startAccount = plugin.gateway.startAccount
      │
      ├─ for each accountId in plugin.config.listAccountIds(cfg):
      │   ├─ 检查是否已启动 (store.tasks.has(id))
      │   ├─ 解析账号配置: plugin.config.resolveAccount(cfg, id)
      │   ├─ 检查启用状态: plugin.config.isEnabled(account, cfg)
      │   ├─ 检查配置完整: plugin.config.isConfigured(account, cfg)
      │   │
      │   ├─ 创建 AbortController
      │   ├─ 更新运行状态: setRuntime(running: true, lastStartAt: now)
      │   │
      │   └─ startAccount({
      │       cfg, accountId, account,
      │       runtime,
      │       abortSignal: abort.signal,
      │       log: channelLogs[channelId],
      │       getStatus, setStatus,
      │     })
      │     │
      │     └─ (Telegram) → monitorTelegramProvider()
      │                      → Grammy Runner / Webhook Server
      │
      └─ 错误处理:
          ├─ catch → setRuntime(lastError: message)
          └─ finally → setRuntime(running: false, lastStopAt: now)
```

### 运行时状态追踪

```typescript
type ChannelRuntimeStore = {
  aborts: Map<string, AbortController>;    // 每个账号的中止控制器
  tasks: Map<string, Promise<unknown>>;     // 每个账号的运行任务
  runtimes: Map<string, ChannelAccountSnapshot>; // 每个账号的状态快照
};
```

`getRuntimeSnapshot()` 聚合所有 channel 的账号状态，用于 UI 展示和健康监控。

---

## 11. 设计亮点与可借鉴之处

### 11.1 Adapter 模式

每个 Channel 只需实现必要的 Adapter，无需实现全部。这种**可选 Adapter 组合**模式比传统的继承/全量接口更灵活:

```typescript
// 最简 Channel 实现只需要:
{
  id: "my-channel",
  meta: { ... },
  capabilities: { chatTypes: ["direct"] },
  config: { listAccountIds, resolveAccount }, // 必选
  outbound: { sendText, sendMedia },          // 发消息
  gateway: { startAccount },                  // 生命周期
}
```

### 11.2 插件发现的层级优先级

```
config > workspace > global > bundled
```

允许用户在项目级、全局级、以及内置级别分别管理插件，高优先级覆盖低优先级。

### 11.3 Session Key 的灵活设计

通过 `dmScope` 控制 DM 会话的隔离粒度:
- `"main"` — 所有 DM 共享一个 session (跨渠道统一上下文)
- `"per-peer"` — 每个联系人独立 session
- `"per-channel-peer"` — 每个渠道+联系人独立
- `"per-account-channel-peer"` — 最细粒度

配合 `identityLinks` 实现跨渠道身份关联。

### 11.4 流式输出的三级模式

```
"off"     — 完整回复后一次性发送
"partial" — Token 级实时编辑消息 (Telegram editMessageText)
"block"   — 语义块级流式 (一段完成后发送)
```

### 11.5 安全模型分层

```
DM 层:   dmPolicy (pairing/allowlist/open/disabled)
Group 层: groupPolicy (open/allowlist/disabled)
提及层:   mention-gating (requireMention + 检测)
命令层:   command-gating (权限控制)
```

### 11.6 统一的 Outbound 投递管线

所有 Channel 共享同一个 `deliverOutboundPayloads()` 入口，通过 `loadChannelOutboundAdapter()` 动态加载具体 Channel 的发送逻辑。文本分块、Payload 标准化、错误处理、transcript 镜像等逻辑全部复用。

### 11.7 值得注意的工程实践

- **Update Offset 持久化** — Telegram 轮询重启后从上次 offset 恢复，避免重复处理
- **媒体组缓冲** — 解决 Telegram 多图消息拆分为多个 update 的问题
- **文本片段重组** — 解决超长消息被 Telegram 拆分的问题
- **Grammy sequentialize** — 保证同一聊天的消息按顺序处理
- **AbortController** — 优雅的生命周期控制
- **Symbol.for 全局单例** — 跨模块共享插件注册表

---

## 12. 关键文件索引

### Inbound 链路
| 文件 | 行数 | 职责 |
|------|------|------|
| `src/telegram/monitor.ts` | 215 | 长轮询/Webhook 启动入口 |
| `src/telegram/webhook.ts` | 127 | HTTP Webhook 服务器 |
| `src/telegram/bot.ts` | 494 | Grammy Bot 创建与中间件编排 |
| `src/telegram/bot-handlers.ts` | 928 | 消息/回调/反应处理器注册 |
| `src/telegram/bot-message.ts` | 92 | 消息处理器工厂 |
| `src/telegram/bot-message-context.ts` | 700 | Inbound 上下文构建 |
| `src/telegram/bot-message-dispatch.ts` | 357 | 调度到 Agent 并处理流式回复 |

### Outbound 链路
| 文件 | 行数 | 职责 |
|------|------|------|
| `src/infra/outbound/deliver.ts` | 376 | 主投递编排 |
| `src/infra/outbound/payloads.ts` | ~150 | Payload 标准化 |
| `src/infra/outbound/channel-selection.ts` | ~100 | 多 Channel 选择 |
| `src/telegram/send.ts` | 754 | Telegram 发送函数 |
| `src/telegram/bot/delivery.ts` | 562 | Telegram 回复投递 |

### 插件系统
| 文件 | 行数 | 职责 |
|------|------|------|
| `src/plugins/registry.ts` | ~350 | 插件注册表 |
| `src/plugins/runtime.ts` | ~50 | 全局单例管理 |
| `src/plugins/loader.ts` | ~400 | 插件加载器 |
| `src/channels/plugins/types.plugin.ts` | 85 | ChannelPlugin 接口 |
| `src/channels/plugins/types.adapters.ts` | 313 | Adapter 接口 |
| `src/channels/plugins/types.core.ts` | 332 | 基础类型 |
| `src/channels/plugins/catalog.ts` | ~300 | 插件发现与目录 |

### 路由与会话
| 文件 | 行数 | 职责 |
|------|------|------|
| `src/routing/resolve-route.ts` | 261 | Agent 路由解析 |
| `src/routing/bindings.ts` | 121 | 路由绑定 |
| `src/routing/session-key.ts` | 250 | Session Key 构建 |

### 生命周期
| 文件 | 行数 | 职责 |
|------|------|------|
| `src/gateway/server-channels.ts` | 309 | ChannelManager |
| `src/channels/registry.ts` | 180 | 内置 Channel 注册表 |
| `extensions/telegram/index.ts` | ~15 | 插件入口 |
| `extensions/telegram/src/channel.ts` | 482 | Telegram ChannelPlugin 实现 |

### 配置
| 文件 | 职责 |
|------|------|
| `src/config/types.channels.ts` | Channel 配置汇总 |
| `src/config/types.telegram.ts` | Telegram 专属配置 (~200 行) |
