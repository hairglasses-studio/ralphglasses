# 10 -- MCP Ecosystem Analysis

Generated: 2026-04-04

## 1. MCP Specification Status

### Current Version: 2025-11-25

The MCP specification is now on its fourth release, published on the protocol's first anniversary. The spec is governed by the Agentic AI Foundation (AAIF) under the Linux Foundation, co-founded by Anthropic, OpenAI, Google, Microsoft, AWS, Block, Cloudflare, and Bloomberg in December 2025. Anthropic donated the protocol but individual projects retain full technical autonomy under the AAIF structure.

**Spec version timeline:**

| Version | Key additions |
|---------|--------------|
| 2024-11-05 | Initial release. stdio + HTTP+SSE transports, tools/resources/prompts primitives |
| 2025-03-26 | Streamable HTTP transport (replaces HTTP+SSE). OAuth 2.1 authorization framework |
| 2025-06-18 | Resource Indicators (RFC 8707) required for clients. Servers classified as OAuth Resource Servers. Incremental scope consent |
| 2025-11-25 | Tasks primitive (async call-now-fetch-later). Extensions framework. OAuth Client ID Metadata Documents. OpenID Connect Discovery 1.0. JSON Schema 2020-12 default. Icon metadata for tools/resources/prompts |

Sources:
- [MCP Spec 2025-11-25](https://modelcontextprotocol.io/specification/2025-11-25)
- [First Anniversary Blog Post](https://blog.modelcontextprotocol.io/posts/2025-11-25-first-mcp-anniversary/)
- [Key Changes Changelog](https://modelcontextprotocol.io/specification/2025-11-25/changelog)
- [WorkOS: MCP 2025-11-25 Spec Update](https://workos.com/blog/mcp-2025-11-25-spec-update)
- [Auth0: MCP Specs Update -- All About Auth](https://auth0.com/blog/mcp-specs-update-all-about-auth/)
- [AAIF Formation Announcement](https://www.linuxfoundation.org/press/linux-foundation-announces-the-formation-of-the-agentic-ai-foundation)
- [Anthropic: Donating MCP](https://www.anthropic.com/news/donating-the-model-context-protocol-and-establishing-of-the-agentic-ai-foundation)

### Transport Options

| Transport | Status | Use case |
|-----------|--------|----------|
| **stdio** | Stable, recommended for local | Local process communication. No network overhead. ralphglasses uses this exclusively |
| **Streamable HTTP** | Stable since 2025-03-26 | Remote servers. Single POST endpoint (e.g., `/mcp`). Optional GET for server-initiated messages. Replaces old dual-endpoint SSE |
| **HTTP+SSE** | Deprecated since 2025-03-26 | Legacy. Two endpoints (GET `/sse` + POST). Fragile during network drops, hard to recover sessions. SDKs dropping support |

SSE was deprecated because it was unreliable during network interruptions, sessions were hard to recover, and the dual-endpoint model fought with load balancers and horizontal scaling. Streamable HTTP consolidates to a single endpoint and supports standard HTTP infrastructure.

Sources:
- [MCP Transports Spec](https://modelcontextprotocol.io/specification/2025-03-26/basic/transports)
- [Why MCP Deprecated SSE](https://blog.fka.dev/blog/2025-06-06-why-mcp-deprecated-sse-and-go-with-streamable-http/)
- [Auth0: Why Streamable HTTP Simplifies Security](https://auth0.com/blog/mcp-streamable-http/)
- [Cloudflare: MCP Transport](https://developers.cloudflare.com/agents/model-context-protocol/transport/)

### Authorization Model

The 2025-06-18 spec established MCP's auth story:

- MCP servers are OAuth 2.1 Resource Servers
- Clients must implement Resource Indicators (RFC 8707) to prevent malicious servers from obtaining tokens scoped to other services
- OpenID Connect Discovery 1.0 for authorization server discovery (2025-11-25)
- OAuth Client ID Metadata Documents as recommended client registration (2025-11-25)
- Incremental scope consent via WWW-Authenticate headers

For stdio transport (ralphglasses' current mode), auth is not applicable -- the trust boundary is the OS process. Auth becomes relevant only when exposing tools over HTTP.

Sources:
- [MCP Authorization Tutorial](https://modelcontextprotocol.io/docs/tutorials/security/authorization)
- [Stack Overflow: Auth in MCP](https://stackoverflow.blog/2026/01/21/is-that-allowed-authentication-and-authorization-in-model-context-protocol/)
- [November 2025 Auth Spec Details](https://den.dev/blog/mcp-november-authorization-spec/)
- [Aaron Parecki: Client Registration](https://aaronparecki.com/2025/11/25/1/mcp-authorization-spec-update)

### 2026 Roadmap Priorities

The core maintainers ranked these as the top four focus areas for 2026:

1. **Transport evolution** -- Horizontal scaling without server-side session state; `.well-known` metadata for capability discovery without live connections
2. **Tasks primitive** -- Maturing the experimental async tasks from 2025-11-25 into a stable feature
3. **Enterprise readiness** -- Standardized audit trails, SSO-integrated auth (replacing static secrets), gateway behavior specification
4. **Governance model** -- SEP process refinement under AAIF

Three protocol-level primitives remain missing: identity propagation, adaptive tool budgeting, and structured error semantics.

Sources:
- [2026 MCP Roadmap](http://blog.modelcontextprotocol.io/posts/2026-mcp-roadmap/)
- [The New Stack: MCP Production Pain Points](https://thenewstack.io/model-context-protocol-roadmap-2026/)
- [WorkOS: Enterprise Readiness Priority](https://workos.com/blog/2026-mcp-roadmap-enterprise-readiness)
- [MCP Official Roadmap Page](https://modelcontextprotocol.io/development/roadmap)

---

## 2. Go SDK Landscape

There are now three Go MCP implementations that matter:

### Feature Comparison

| Feature | mcp-go (mark3labs) | Official go-sdk (modelcontextprotocol) | mcpkit (hairglasses-studio) |
|---------|-------------------|---------------------------------------|---------------------------|
| **Import path** | `github.com/mark3labs/mcp-go` | `github.com/modelcontextprotocol/go-sdk` | `github.com/hairglasses-studio/mcpkit` |
| **Maintainers** | Mark III Labs (Ed Zynda + community) | MCP org + Google collaboration | hairglasses-studio (internal) |
| **Version** | v0.46.0 (ralphglasses pinned) | v1.4.0+ (stable since v1.0.0) | Internal (35 packages, 700+ tests) |
| **GitHub stars** | ~8.4k | Growing (official backing) | Private |
| **Spec compliance** | 2025-11-25 (with backward compat) | 2025-11-25 | 2025-03-26 (assumed, per hg-mcp SSE deprecation) |
| **Transports** | stdio, Streamable HTTP, SSE, in-process | stdio, Streamable HTTP | stdio (primary), SSE/HTTP via shim |
| **OAuth support** | Community-contributed | Built-in `auth` package | Not applicable (local only) |
| **API stability** | Pre-1.0, breaking changes possible | v1.0+ semver guarantee | Internal contract |
| **ToolModule pattern** | No (flat handlers) | No (flat handlers) | Yes (`Name/Description/Tools` interface) |
| **Middleware** | Basic | Basic | Full chain (OTel, Prometheus, audit, rate limit, circuit breaker, timeout, panic recovery) |
| **Registry/discovery** | No | No | Yes (`DynamicRegistry`, deferred loading, gateway) |
| **Lazy initialization** | No | No | Yes (`LazyClient[T]`) |
| **Thread safety patterns** | User responsibility | User responsibility | Built-in (`sync.RWMutex` conventions, `sync.Once`) |

**Key takeaway**: mcp-go and the official go-sdk are protocol-level SDKs. mcpkit is a production application framework built on top of mcp-go. They serve different layers of the stack.

Sources:
- [mark3labs/mcp-go GitHub](https://github.com/mark3labs/mcp-go)
- [Official go-sdk GitHub](https://github.com/modelcontextprotocol/go-sdk)
- [Official go-sdk v1.0.0 Release](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.0.0)
- [Go SDK Design Discussion](https://github.com/orgs/modelcontextprotocol/discussions/364)
- [go-sdk pkg.go.dev](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp)

---

## 3. Transport Evolution

### Current State: stdio Only

ralphglasses uses stdio exclusively. This is correct for the current use case: Claude Code (and Gemini CLI, Codex CLI) spawn ralphglasses as a local subprocess. There is no network boundary.

### When HTTP Transport Becomes Necessary

| Scenario | Requires HTTP? | Timeline |
|----------|---------------|----------|
| Local CLI agent sessions | No -- stdio is optimal | Now |
| Thin client (distro) booting into TUI | No -- same machine | Now |
| Remote agent sessions (fleet across machines) | Yes | When fleet goes multi-host |
| MCP Registry publishing / `.well-known` discovery | Yes (metadata only) | When registering tools externally |
| Multi-user fleet (team scenario) | Yes + OAuth | Not on current roadmap |
| Gateway aggregation across network | Yes | When consolidating remote servers |

### Migration Risk Assessment

**Low risk for stdio users**: The spec explicitly maintains stdio as a first-class transport. It is not deprecated and is recommended for local integrations.

**Medium risk for future HTTP adoption**: hg-mcp already supports Streamable HTTP, proving the mcpkit shim can handle the transition. ralphglasses would need:
1. An HTTP listener in the `mcp` command path
2. Session management (the spec's stateful session model)
3. OAuth if multi-user

**Recommendation**: Do not add HTTP transport now. Plan for it when multi-host fleet or external tool publishing becomes a requirement. The mcpkit shim architecture means the change is localized to transport setup, not to individual tool handlers.

Sources:
- [Cloudflare: MCP Transport](https://developers.cloudflare.com/agents/model-context-protocol/transport/)
- [Sunpeak: SSE to Streamable HTTP Migration](https://sunpeak.ai/blogs/claude-connector-sse-to-streamable-http/)

---

## 4. Auth Story for Multi-User Fleet

### Current State: No Auth Needed

With stdio transport, trust is inherited from the OS process model. The user running `claude mcp add ralphglasses -- go run . mcp` is the only user. The 166 tools run with that user's permissions.

### What Multi-User Fleet Would Require

If ralphglasses ever exposes tools to remote agents or multiple users:

1. **OAuth 2.1 Resource Server**: ralphglasses becomes an OAuth Resource Server per the MCP spec. Each tool invocation carries a bearer token.
2. **Resource Indicators**: Clients must send RFC 8707 resource indicators to scope tokens to the ralphglasses server specifically.
3. **Identity propagation**: The 2026 roadmap identifies this as a missing primitive. Currently, audit logs show service accounts, not the human who triggered the action. For fleet scenarios with 166 tools, this is a compliance gap.
4. **Per-namespace authorization**: The 16 namespaces map naturally to OAuth scopes (e.g., `ralph:fleet`, `ralph:session`, `ralph:advanced`). This would allow fine-grained access control.
5. **Gateway token forwarding**: If a gateway aggregates ralphglasses with other servers, the spec needs to define how tokens propagate through intermediaries. This is on the 2026 roadmap as "Cross-App Access."

**Recommendation**: No action needed now. When multi-user scenarios arise, adopt the official go-sdk's `auth` package rather than building custom OAuth flows. The namespace-to-scope mapping is a natural fit.

Sources:
- [MCP OAuth Implementation Guide](https://www.mcpserverspot.com/learn/architecture/mcp-oauth-implementation-guide)
- [SAP: 8 Critical MCP Enterprise Pain Points](https://community.sap.com/t5/artificial-intelligence-blogs-posts/8-critical-pain-points-of-mcp-in-an-enterprise/ba-p/14303370)

---

## 5. Gateway Patterns

### Industry Landscape

MCP gateways have emerged as a major pattern in 2026. Key players:

| Gateway | Type | Notable features |
|---------|------|-----------------|
| **Bifrost** (Maxim AI) | Open-source Go gateway | Dual-role: MCP client + MCP server. Single `/mcp` endpoint aggregates 20+ upstream servers |
| **Microsoft MCP Gateway** | Kubernetes-native | Session-aware stateful routing, lifecycle management |
| **Envoy AI Gateway** | Envoy extension | First-class MCP support in the standard Envoy proxy |
| **Traefik Hub** | API gateway extension | MCP reverse proxy with auth and rate limiting |
| **mcpkit Gateway** (hairglasses) | In-process Go | `DynamicRegistry` + upstream aggregation. Used by claudekit |

A gateway aggregates `tools/list` responses from multiple upstream servers into a unified catalog. Clients send a single request and receive merged results.

Sources:
- [5 Best MCP Gateways (Maxim AI)](https://www.getmaxim.ai/articles/5-best-mcp-gateways-for-developers-in-2026-2/)
- [Composio: MCP Gateways Guide](https://composio.dev/content/mcp-gateways-guide)
- [Microsoft MCP Gateway](https://github.com/microsoft/mcp-gateway)
- [Envoy AI Gateway MCP Support](https://aigateway.envoyproxy.io/blog/mcp-implementation/)
- [Traefik Hub MCP](https://doc.traefik.io/traefik-hub/mcp-gateway/mcp)
- [Agentic Community MCP Gateway Registry](https://github.com/agentic-community/mcp-gateway-registry)

### How Should ralphglasses Aggregate 16 Namespaces?

**Current model**: Single server, deferred loading. All 16 namespaces live in one Go binary. The `ralphglasses_load_tool_group` meta-tool loads namespaces on demand.

**Alternative models considered**:

| Model | Pros | Cons | Verdict |
|-------|------|------|---------|
| **Single server + deferred loading** (current) | Simple deployment. No network hops. Fast tool calls. | All 166 tools in one process. No independent scaling. | Keep for now |
| **Gateway + N namespace servers** | Independent scaling. Namespace isolation. Fault isolation. | N processes to manage. IPC overhead. Complex deployment for a local TUI. | Overkill for local use |
| **Gateway for remote, single server for local** | Best of both worlds. Local stays fast, remote gets proper aggregation. | Two code paths to maintain. | Consider when multi-host fleet arrives |
| **hg-mcp style domain gateways** | Reduces 166 tools to ~10 entry points. Token-efficient. | Loses granular tool semantics. Gateway tools are less discoverable. | Good for external consumers, not for the TUI itself |

**Recommendation**: Keep the current single-server + deferred loading model. It is correct for a local TUI. If ralphglasses ever needs to serve remote clients, add a Streamable HTTP listener and let external gateways (Bifrost, Envoy, or mcpkit Gateway) handle aggregation. Do not split namespaces into separate processes.

---

## 6. Scalability Analysis

### Context Window Consumption at 166 Tools

Tool definitions consume approximately 100-300 tokens each, depending on parameter count and description length. At 166 tools:

| Scenario | Tools in context | Est. tokens | % of 200k Claude context |
|----------|-----------------|-------------|--------------------------|
| All tools loaded eagerly | 166 | 16,600-49,800 | 8-25% |
| Deferred: core only | 12 (10 core + 2 meta) | 1,200-3,600 | 0.6-1.8% |
| Deferred: core + 1 namespace | 12 + ~10 avg | 2,200-6,600 | 1.1-3.3% |
| Deferred: core + 3 namespaces | 12 + ~30 | 4,200-12,600 | 2.1-6.3% |

### Client-Side Tool List Overhead

Research shows that beyond 50 tools, response times increase 2-3x, and beyond 100 tools, 4-5x. Tool selection accuracy degrades significantly past 5-7 tools without specialized filtering.

**ralphglasses' deferred loading is already the right answer**. Loading 12 tools at startup (core + meta-tools) keeps the overhead under 2% of context and maintains high tool selection accuracy. The LLM loads additional namespaces only when it determines it needs them.

### Comparison with Other Org Servers

| Server | Total tools | Strategy | Startup tokens |
|--------|------------|----------|----------------|
| systemd-mcp | 10 | Eager (all) | ~1,500 |
| tmux-mcp | 9 | Eager (all) | ~1,350 |
| process-mcp | 8 | Eager (all) | ~1,200 |
| dotfiles-mcp | 86 | Eager (all) | ~12,900 |
| **ralphglasses** | **166** | **Deferred** | **~1,800** |
| hg-mcp | 1,190+ | Progressive discovery | ~2,000 |
| mesmer | 1,790 | Lazy loading | ~2,000 |

### Stdio-Specific Limits

Stdio has no inherent connection limit or throughput ceiling for the tool counts ralphglasses uses. The bottleneck is context window consumption, not transport. JSON-RPC over stdio handles 166 tool schemas trivially. The real constraint is how many tools the LLM can reason about simultaneously, which is a model limitation, not a protocol limitation.

### Potential Improvements

1. **Tool RAG**: Embed tool descriptions as vectors, retrieve top-k relevant tools per query instead of loading entire namespaces. Research shows this can improve selection accuracy from 13% to 43% while cutting tokens by 50%. This is the emerging industry pattern for large tool surfaces.
2. **Adaptive budgeting**: The 2026 roadmap identifies "adaptive tool budgeting" as a missing primitive. When the spec adds it, ralphglasses should adopt it.
3. **Tool description optimization**: Trim descriptions to essentials. Each token saved across 166 tools compounds.

Sources:
- [Jenova AI: Tool Overload](https://www.jenova.ai/en/resources/mcp-tool-scalability-problem)
- [Lunar: Prevent MCP Tool Overload](https://www.lunar.dev/post/why-is-there-mcp-tool-overload-and-how-to-solve-it-for-your-ai-agents)
- [apxml: Scale MCP to 100+ Tools](https://apxml.com/posts/scaling-mcp-with-tool-rag)
- [RAG-MCP Paper](https://arxiv.org/html/2505.03275v1)
- [Tetrate: MCP Tool Filtering](https://tetrate.io/learn/ai/mcp/tool-filtering-performance)
- [Apideck: MCP Eating Context Window](https://www.apideck.com/blog/mcp-server-eating-context-window-cli-alternative)
- [Gemini CLI Tool Limit Issue](https://github.com/google-gemini/gemini-cli/issues/21823)
- [Writer: RAG for MCP](https://writer.com/engineering/rag-mcp/)
- [Cursor 40-Tool Limit](https://medium.com/@sakshiaroraresearch/cursors-40-tool-tango-navigating-mcp-limits-213a111dc218)

---

## 7. mcp-go Dependency Risk

### Current Situation

ralphglasses depends on `github.com/mark3labs/mcp-go v0.46.0`. This is a community SDK, not the official one.

### Risk Assessment

| Factor | Assessment | Detail |
|--------|-----------|--------|
| **Maintenance activity** | Active | Multiple 2026 releases (v0.45.0 March 2026, v0.46.0 pinned). Active contributors including @ezynda3 (maintainer), @aradyaron, @yuehaii |
| **Community size** | Strong | 8.4k GitHub stars, 795 forks |
| **Spec compliance** | Current | Supports 2025-11-25 with backward compat for all prior versions |
| **API stability** | Risk | Pre-1.0 semver. Breaking changes between minor versions are possible |
| **Bus factor** | Medium | Primary maintainer (Ed Zynda / Mark III Labs) + growing contributor base |
| **Official SDK competition** | High risk | The official `modelcontextprotocol/go-sdk` hit v1.0.0 and is maintained with Google collaboration. It has semver stability guarantees that mcp-go lacks |

### The Official SDK Factor

The official Go SDK (`github.com/modelcontextprotocol/go-sdk`) reached v1.0.0 with a compatibility guarantee and is now at v1.4.0+. It supports the 2025-11-25 spec, includes a built-in `auth` package for OAuth, and has `jsonrpc` package for custom transports.

The official SDK was explicitly inspired by mcp-go but diverged to keep APIs minimal and spec-aligned. It is maintained by the MCP organization in collaboration with Google, giving it institutional backing that no community SDK can match.

**However**, ralphglasses does not depend on mcp-go directly for most of its functionality. The mcpkit framework provides the abstraction layer. The dependency chain is:

```
ralphglasses -> mcpkit -> mcp-go
```

If mcp-go needs to be replaced, the change is isolated to mcpkit's internals. ralphglasses tool handlers, middleware, and registry patterns are mcpkit APIs, not mcp-go APIs.

### Migration Path

1. **Short term (now)**: No action. mcp-go v0.46.0 is current and functional.
2. **Medium term (Q3 2026)**: Evaluate migrating mcpkit's protocol layer from mcp-go to the official go-sdk. This is a mcpkit-internal change that should not affect downstream repos (ralphglasses, hg-mcp, claudekit, etc.) due to the shim architecture.
3. **Trigger for migration**: If mcp-go falls behind on spec compliance, or if the official SDK adds features mcpkit needs (especially the `auth` package for multi-user scenarios).

Sources:
- [mark3labs/mcp-go Releases](https://github.com/mark3labs/mcp-go/releases)
- [Official go-sdk Releases](https://github.com/modelcontextprotocol/go-sdk/releases)
- [Official Go SDK for MCP Announcement](https://socket.dev/blog/official-go-sdk-for-mcp)
- [Go SDK Design Discussion](https://github.com/orgs/modelcontextprotocol/discussions/364)

---

## 8. Competing Protocols

### Protocol Landscape (April 2026)

| Protocol | Owner | Purpose | Governance |
|----------|-------|---------|-----------|
| **MCP** | AAIF (Linux Foundation) | Agent-to-tool connectivity | Open spec, SEP process, AAIF board |
| **A2A** | Google (AAIF) | Agent-to-agent communication | Open spec under AAIF |
| **AGENTS.md** | OpenAI (AAIF) | Agent discovery via well-known files | Under AAIF |
| **OpenAI function calling** | OpenAI (proprietary) | Native tool integration | Proprietary API |
| **Claude tool_use** | Anthropic (proprietary) | Native tool integration | Proprietary API |
| **ACP** | Various | Agent communication protocol | Community |

### MCP vs A2A: Complementary, Not Competing

The industry consensus is clear: MCP and A2A solve different problems and are designed to work together.

- **MCP** is the "USB-C port for AI" -- it connects agents to tools, databases, and APIs. Vertical integration.
- **A2A** is agent-to-agent communication -- it enables task delegation, lifecycle management, and peer coordination across distributed agents. Horizontal coordination.

Both are now under the AAIF, meaning neither Anthropic nor Google solely controls the specs. The governance structure ensures interoperability is a shared goal.

### Relevance to ralphglasses

| Protocol | ralphglasses relevance | Action needed |
|----------|----------------------|---------------|
| **MCP** | Core dependency. All 166 tools use MCP. | Continue tracking spec evolution |
| **A2A** | Directly relevant to multi-agent fleet coordination. The `fleet_h` namespace already has `ralphglasses_a2a_offers`. | Monitor A2A spec maturity. Current implementation is ahead of the protocol. |
| **AGENTS.md** | Could enable agent discovery across repos. | Low priority. ralphglasses already scans `~/hairglasses-studio/` for repos. |
| **Native function calling** | Used internally by Claude/Gemini/Codex when processing tool results. MCP provides the transport; native function calling provides the model integration. | No conflict. These are different layers. |

### Will MCP Be Displaced?

No. MCP has reached escape velocity:

- 10,000+ active servers
- 97 million monthly SDK downloads
- Adopted by all major AI companies (Anthropic, OpenAI, Google, Microsoft, AWS)
- Linux Foundation governance ensures no single vendor can kill it
- 6,000+ servers on Smithery marketplace alone

The risk is not displacement but fragmentation -- too many registries, inconsistent server quality, and spec versions diverging across implementations. For ralphglasses, the mitigation is the mcpkit abstraction layer.

Sources:
- [A2A vs MCP Guide](https://aimojo.io/a2a-vs-mcp/)
- [Auth0: MCP vs A2A](https://auth0.com/blog/mcp-vs-a2a/)
- [Cisco: Network Engineer's Mental Model](https://blogs.cisco.com/ai/mcp-and-a2a-a-network-engineers-mental-model-for-agentic-ai)
- [Koyeb: AI Agent Protocol Wars](https://www.koyeb.com/blog/a2a-and-mcp-start-of-the-ai-agent-protocol-wars)
- [Protocol Ecosystem Map 2026](https://www.digitalapplied.com/blog/ai-agent-protocol-ecosystem-map-2026-mcp-a2a-acp-ucp)
- [GitHub Blog: MCP Joins Linux Foundation](https://github.blog/open-source/maintainers/mcp-joins-the-linux-foundation-what-this-means-for-developers-building-the-next-era-of-ai-tools-and-agents/)
- [MCP Joins AAIF Blog](https://blog.modelcontextprotocol.io/posts/2025-12-09-mcp-joins-agentic-ai-foundation/)

---

## 9. Marketplace and Discovery

### Current Ecosystem

| Registry | Scale | Focus |
|----------|-------|-------|
| **Smithery** | 6,000+ servers | Largest marketplace. Keyword + NL search. Frameworks like CAMEL query it natively |
| **Official MCP Registry** | Growing | Backed by MCP working group. `registry.modelcontextprotocol.io` |
| **MCP Market** | Curated | Premium/commercial focus. SLA-backed servers |
| **awesome-mcp-servers** | GitHub repo | Still one of the highest-traffic discovery surfaces |
| **npm** | Package registry | TypeScript servers discoverable via npm search |

### Discovery Gap

Tool discovery in the MCP ecosystem remains fundamentally broken as of April 2026. Key issues:

- Semantic search cannot distinguish a good tool from a good description
- Tool descriptions function as advertisements, not specifications
- No standard quality scoring or certification
- `.well-known` metadata for capability discovery is on the 2026 roadmap but not yet standardized

### Relevance to ralphglasses

ralphglasses is an internal orchestration tool, not a publicly discoverable server. Its tools are consumed by Claude Code, Gemini CLI, and Codex CLI in local sessions. The marketplace/registry ecosystem is not directly relevant today.

However, if ralphglasses tools are ever published externally (e.g., as part of the open-source release), Smithery and the Official MCP Registry would be the targets. The existing `ralphglasses_tool_groups` and `ralphglasses_load_tool_group` meta-tools already implement the discovery pattern that registries recommend.

Sources:
- [Smithery AI (WorkOS)](https://workos.com/blog/smithery-ai)
- [Official MCP Registry](https://registry.modelcontextprotocol.io/)
- [Getting Found by Agents: Builder's Guide](https://blog.icme.io/getting-found-by-agents-a-builders-guide-to-tool-discovery-in-2026/)
- [Composio: Smithery Alternatives](https://composio.dev/content/smithery-alternative)

---

## 10. Recommendations

Prioritized list of MCP-related actions for ralphglasses, ordered by impact and urgency.

### P0 -- Do Now

1. **Keep deferred loading active and tuned**. The current 12-tool startup (10 core + 2 meta) is the right answer for a 166-tool server. Review tool descriptions for token efficiency -- every token saved is multiplied by every invocation across all sessions.

2. **Pin mcp-go and track releases**. The current v0.46.0 pin is correct. Subscribe to `mark3labs/mcp-go` releases. If a release breaks the API, the mcpkit shim isolates the blast radius.

### P1 -- Plan for Q3 2026

3. **Evaluate mcpkit migration from mcp-go to official go-sdk**. The official SDK has semver stability guarantees (v1.0+), Google co-maintenance, and a built-in `auth` package. The migration is a mcpkit-internal change. Trigger: when mcpkit needs OAuth or when mcp-go falls behind on spec compliance.

4. **Adopt MCP 2025-11-25 spec features selectively**. The Tasks primitive (async call-now-fetch-later) maps directly to fleet job submission. The Extensions framework could formalize ralphglasses' namespace-loading pattern. Evaluate after mcpkit adds support.

5. **Implement Tool RAG for dynamic tool selection**. Embed tool descriptions as vectors, retrieve top-k per query. This would reduce namespace-level loading to query-level loading, improving selection accuracy from ~13% to ~43% per research. This is more impactful than adding new tools.

### P2 -- Plan When Triggered

6. **Add Streamable HTTP transport when multi-host fleet arrives**. Not needed for local TUI. The mcpkit shim means this is a transport-layer change, not a handler rewrite. hg-mcp already proves the pattern works.

7. **Implement OAuth when multi-user scenarios arise**. Use the official go-sdk `auth` package. Map namespaces to OAuth scopes. Do not build custom auth.

8. **Publish to MCP Registry and Smithery when open-sourced**. The deferred loading meta-tools already satisfy the discovery pattern. Add `.well-known` metadata when the spec standardizes it.

### P3 -- Monitor

9. **Track A2A protocol maturity**. The `fleet_h` namespace's `a2a_offers` tool is ahead of the protocol. As A2A stabilizes under AAIF, ensure compatibility. A2A is for agent-to-agent coordination; MCP is for agent-to-tool. ralphglasses sits at the intersection.

10. **Watch for adaptive tool budgeting in the spec**. When MCP adds this primitive, adopt it to let clients declare how many tools they can handle. This would let ralphglasses automatically adjust deferred loading thresholds per client capability.

11. **Monitor gateway standardization**. The 2026 roadmap includes gateway behavior specification (token propagation, session semantics). When standardized, ensure mcpkit Gateway is compliant.

---

## Appendix A: MCP Ecosystem Statistics (April 2026)

| Metric | Value | Source |
|--------|-------|--------|
| Active MCP servers | 10,000+ | [MCP Roadmap Blog](http://blog.modelcontextprotocol.io/posts/2026-mcp-roadmap/) |
| Monthly SDK downloads | 97 million | [Linux Foundation Announcement](https://www.linuxfoundation.org/press/linux-foundation-announces-the-formation-of-the-agentic-ai-foundation) |
| Smithery listings | 6,000+ | [Composio: Smithery Alternatives](https://composio.dev/content/smithery-alternative) |
| Spec versions | 4 (2024-11-05, 2025-03-26, 2025-06-18, 2025-11-25) | [MCP Spec Changelog](https://modelcontextprotocol.io/specification/2025-11-25/changelog) |
| Official SDKs | TypeScript, Python, Go (+ community: Java, C#, Ruby, Kotlin, Rust) | [go-sdk GitHub](https://github.com/modelcontextprotocol/go-sdk) |
| AAIF co-founders | 6 (OpenAI, Anthropic, Google, Microsoft, AWS, Block) | [AAIF Formation](https://www.linuxfoundation.org/press/linux-foundation-announces-the-formation-of-the-agentic-ai-foundation) |
| Governance model | SEP (Spec Enhancement Proposals) under AAIF | [MCP Blog](https://blog.modelcontextprotocol.io/) |

## Appendix B: ralphglasses MCP Dependency Chain

```
ralphglasses (166 tools, 16 namespaces)
  -> mcpkit (35 packages, 700+ tests)
       -> mark3labs/mcp-go v0.46.0 (protocol layer)
            -> MCP spec 2025-11-25
```

Migration to official go-sdk would change only the bottom layer:

```
ralphglasses (unchanged)
  -> mcpkit (shim update only)
       -> modelcontextprotocol/go-sdk v1.4.0+ (protocol layer)
            -> MCP spec 2025-11-25
```

No tool handler, middleware, or registry code changes required in ralphglasses or any other downstream repo.
