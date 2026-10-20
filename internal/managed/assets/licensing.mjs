// SPDX-License-Identifier: AGPL-3.0-only
// Project attribution: Xpotato.net / Xpotato1024; third-party rights retained.
// Offers public source information through MCP without writing to stdout.
export const sourceTool = 'xpotato_tunnel_source';
export function offerSource(Server, info) {
  const text = JSON.stringify(info, null, 2);
  const notice = `Xpotato.net / Xpotato1024 — xpotato-mcp-tunnel (${info.license}). Corresponding source and license: call ${sourceTool}. ${info.source || info.notice}`;
  const original = Server.prototype.setRequestHandler;
  Server.prototype.setRequestHandler = function(schema, handler) {
    const method = schema?.shape?.method?.value;
    if (method === 'initialize') {
      return original.call(this, schema, async (...args) => {
        const result = await handler(...args);
        return { ...result, instructions: [result.instructions, notice].filter(Boolean).join('\n\n') };
      });
    }
    if (method === 'tools/list') {
      return original.call(this, schema, async (...args) => {
        const result = await handler(...args);
        if (args[0]?.params?.cursor != null) return result;
        return { ...result, tools: [...result.tools, {
          name: sourceTool, description: notice,
          inputSchema: { type: 'object', properties: {}, additionalProperties: false },
          annotations: { readOnlyHint: true, destructiveHint: false, idempotentHint: true, openWorldHint: false }
        }] };
      });
    }
    if (method === 'tools/call') {
      return original.call(this, schema, (request, ...args) => {
        if (request.params?.name === sourceTool) return { content: [{ type: 'text', text }] };
        return handler(request, ...args);
      });
    }
    return original.call(this, schema, handler);
  };
}
