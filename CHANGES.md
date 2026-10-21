# 0.2.11 Draft — 2026-10-21

- Keep the Tunnel core in Go; add a thin per-user NSIS installer candidate and
  a shared maintenance menu. No merge, release or target-device installation.
- Normal new install, update, repair and start check only bounded launch files.
  Archive hashes are streamed during acquisition; full expanded-tree comparison
  is explicit `doctor --deep`. This does not guarantee every stored transitive
  dependency remains unchanged between diagnostics.
- Repair creates a new generation without replacing any running generation.
  Separate file/byte progress and report interruption/timeout/failure clearly.
- Group packaged user docs and legal notices without losing source offers.
- Build/test/CI and installer smoke validation are pending the measurement hold.

# 0.2.10 candidate — 2026-10-20

Project attribution and maintenance: **Xpotato.net / Xpotato1024**.
Project-specific code is now offered under **AGPL-3.0-only**. This record does
not change past release conditions or third-party copyright ownership.

Changes prepared with Codex assistance for review:

- Runtime generations are addressed by their recipe; core/runtime selection and
  rollback use a paired transaction. Update progress and interruption handling
  preserve the last verified generation.
- Windows acquisition verifies fixed Release metadata and the GitHub asset
  digest, then safely verifies/extracts the same bytes without executing Setup.
- Core packages include legal notices. A separate corresponding-source archive
  includes the exact tracked source and Go build dependencies with their notices.
- Explicit `about`, `license`, and JSON `source` commands expose attribution,
  licensing and build provenance. MCP initialization and a read-only source tool
  offer the same information without unsolicited stdout text.

Desktop Commander remains **0.2.52**, under its original MIT conditions. The
existing local patch suppresses its preemptive Chrome download at startup;
the new source-offer and activity adapters use its MCP SDK handler boundary.
Node, Commander, Tunnel and npm dependency implementations are separately
retrieved and are not included in these core/source archives. Consult
[docs/licensing.md](docs/licensing.md) and THIRD-PARTY-NOTICES.txt for boundaries.

This is a review candidate, not a record of publication. Downstream modifiers
must add their own accurate changes and dates and update source references.
