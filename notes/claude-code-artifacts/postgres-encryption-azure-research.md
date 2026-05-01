# Postgres Encryption on Azure — Research Plan and Log

Living document. Research questions up top, findings appended as we go, decisions captured at the bottom so we don't relitigate them.

Started: 2026-04-30.

## Terminology — read this before the rest

The word "customer" gets overloaded across three different things in this space. Pin it down once:

| Term | Who holds the key | Granularity | Notes |
|------|-------------------|-------------|-------|
| **SMK** (service-managed key) | Microsoft | One service key, opaque to us | Default for Flex Server. "Customer" not in the picture. |
| **CMK** (customer-managed key, Azure's term) | Us (Accordli, the Azure account holder) | Typically one key for the DB | "Customer" here means *Azure's* customer, i.e. us — not the law firms. |
| **Per-Org / per-tenant keys** (application-layer) | Us, in our Key Vault | One data-encryption key per end-customer Organization | Independent of SMK/CMK. Lives at the envelope-encryption layer. |
| **HYOK / BYOK** | The end customer (law firm), in *their* cloud | One key per end-customer Org, *controlled by them* | Enterprise upsell pattern. Ironclad ships this on a paid tier. |

These are independent axes. Granularity (one key vs per-Org) is orthogonal to who holds them (us vs the end customer).

When a lawyer on a security call asks "is the data encrypted with our key?", they almost always mean **HYOK**, not Azure CMK. Don't conflate the two in questionnaire answers.

**Two independent axes, not one.** Storage-layer encryption (SMK or CMK) decides *who holds the master key for the database volume*. Application-layer envelope encryption decides *whether tenants are cryptographically isolated from each other inside the database*. CMK alone does not give per-tenant isolation — it just changes who holds the one shared key. Per-Org keys live one layer up, in the application, regardless of what's underneath. The two stack; we mix and match independently.

## Scope locked in

These are the scoping answers we agreed on before starting research. Treat as fixed unless explicitly reopened.

| # | Question | Answer |
|---|----------|--------|
| 1 | Threat model | Lost/stolen backup blob; rogue Azure operator. Out of scope: compromised app process, subpoena resistance. |
| 2 | Driver | Self-imposed. Goal is a clean security-questionnaire answer, not a specific customer ask. |
| 3 | BYOK at launch | No. Ship on platform-managed keys. Design data model so envelope encryption can layer in later. |
| 4 | Per-Org keys | Not at storage layer. Yes at application envelope layer when we add it; every encrypted blob carries an `org_key_id`. |
| 5 | Column-level scope | Encrypt contract text, `Finding.details` JSONB, user-provided answers, Prefix. Indexable scalars (type, severity, Lens id, timestamps, FK ids) stay plaintext. No queryable-while-encrypted. |
| 6 | SOC 2 posture | Defaults sufficient for Type I. AES-256 PMK at rest, TLS 1.2+ enforced, automated key rotation, Key Vault diagnostics on. |
| 7 | Budget | Standard Key Vault only. No HSM/Premium until a customer's questionnaire forces FIPS 140-2 Level 3. |

**Load-bearing assumption:** first ~12 months of customers are solo practitioners and small firms whose security bar tops out at "AES-256, TLS, SOC 2 in flight." If we chase AmLaw 100 or regulated in-house teams in year one, #3 and #7 flip.

## Research questions

Priority order. Each gets a Findings subsection below as we work it.

1. **Azure Postgres Flexible Server encryption options today.** PMK vs CMK vs infrastructure double encryption. Which are available on Burstable B2s specifically.
2. **CMK interactions with backups, PITR, replicas, geo-redundancy.** What happens to restore if the key is rotated, revoked, or the source Key Vault is gone.
3. **Key rotation mechanics.** Automatic vs manual, downtime, audit trail, rollback.
4. **Key Vault outage failure mode.** What happens to the database when Key Vault is unreachable.
5. **`pgcrypto` on Flex Server.** Availability, gotchas, performance and index implications for column-level envelope encryption.
6. **TLS posture.** Minimum version supported and enforced, certificate handling, client expectations.
7. **Competitor disclosures.** What Ironclad, Harvey, Spellbook, Hebbia say publicly about Postgres encryption — sets the bar for what lawyers will compare us to.
8. **Published patterns or pitfalls.** Anyone shipping legal AI on Azure Postgres who has written usefully about encryption choices.

## Findings

### 1. Azure Postgres Flexible Server encryption options

**Sources:** MS Learn — [Data Encryption (Flex)](https://learn.microsoft.com/en-us/azure/postgresql/flexible-server/security-data-encryption) (page updated 2026-04-01); MS Learn — [Compute options](https://learn.microsoft.com/en-us/azure/postgresql/compute-storage/concepts-compute); MS Learn — [Infrastructure double encryption (Single Server, deprecated path)](https://learn.microsoft.com/en-us/azure/postgresql/single-server/concepts-infrastructure-double-encryption).

**Bottom line.** Flex Server gives us two modes: service-managed keys (default, AES-256 via Azure Storage encryption) and customer-managed keys (CMK) backed by Key Vault or Key Vault Managed HSM. CMK is set at server creation and cannot be toggled later. Infrastructure double encryption is **not** offered on Flex — that was a Single Server feature only, and Single Server is the deprecated product. There is no third "double encryption" knob for us to turn on Flex.

**Burstable B2s and CMK.** No tier exclusion for CMK is documented on Flex. The known Burstable carve-outs are different things — no HA, no built-in PgBouncer, no VNet integration, no scaling between Burstable and V6 SKUs. CMK is not in that list, and the data-encryption page does not gate CMK by SKU. Treat CMK as available on Burstable B2s, but verify in the portal/CLI when we provision (a five-minute confirmation we should do before locking #3).

**Mode is creation-time only.** The Microsoft doc is explicit: "The mode can only be selected at server creation time. It can't be changed from one mode to another for the lifetime of the server." If we ship on SMK and later want CMK, the migration path is PITR-restore into a new CMK-configured server. Not free, but not a data-model migration either. Reinforces our scoping decision #3 (ship SMK, add CMK later if a customer pulls us).

**What's encrypted under SMK.** All system and user databases, server logs, WAL segments, and backups. AES-256 at the storage layer. The doc says "Data encryption based on service managed keys doesn't negatively affect the performance of your workloads" — true for the storage-layer path, less true if we layer pgcrypto on top (covered in #5).

**Managed HSM option.** If we ever need FIPS 140-3 validated HSM, Flex supports Key Vault Managed HSM as a CMK key store with the same setup pattern. Ignored for now per scoping decision #7, but the path exists.

**Implication for our design.** Default Flex with SMK clears the "AES-256 at rest" questionnaire bar. The "rogue Azure operator" leg of our threat model is not actually addressed by SMK alone — Microsoft holds the key. To address it, we need CMK *or* application-layer envelope encryption with our own keys. Since we punted CMK to later, the envelope encryption path (decision #4 + #5) is what carries the rogue-operator story for v1. Flag this clearly when we get to #5.

### 2. CMK interactions with backups, PITR, replicas

**Sources:** Same MS Learn page as #1.

**Bottom line.** PITR restores and read replicas inherit CMK encryption automatically. Geo-redundancy and replicas across regions add Key Vault setup overhead (one Key Vault per region, separate managed identity per region — you cannot reuse a single user-assigned identity across regions).

**Key gotchas:**
- After a PITR restore, do **not** revoke the original key. The restored server may still depend on it. Microsoft is explicit: "we don't support key revocation after you restore a server with customer managed key to another server."
- Geo-redundant backup requires a Key Vault in the destination region holding the encryption key. ARM API version `2022-11-01-preview` or later is required if we automate this with templates.
- Read replicas need their own region-local Key Vault and identity.

**Implication.** If we adopt CMK later, the operational footprint grows quickly with HA, geo-redundant backup, and replicas: more Key Vaults, more identities, more rotation surface. For a starter Burstable B2s deployment with no replicas this is mostly theoretical, but it's a real "ops cost of CMK" line item to factor in when we evaluate the upgrade.

### 3. Key rotation mechanics

**Sources:** Same MS Learn page.

**Two modes:**
- **Manual key version updates.** URI includes a version GUID. After every rotation in Key Vault, you must update the server's CMK property to point at the new version, or the server eventually moves to Inaccessible. Microsoft's own description: "error-prone work for the operators."
- **Automatic key version updates.** Use a version-less URI. The server picks up new versions automatically. Pairs cleanly with Key Vault auto-rotation policies.

**Recommendation when we adopt CMK:** version-less URI + Key Vault auto-rotation + alerts on key access failures. Manual mode is a footgun and the doc says so plainly.

**Re-encryption window.** When a key rotates, the data encryption key gets re-wrapped. The doc says "most reencryptions should happen within 30 minutes" and recommends keeping the old key version available **at least 2 hours** before disabling it. So rotation isn't instantaneous — there's a soak period to honor.

### 4. Key Vault outage failure mode

**Sources:** Same MS Learn page.

**The blunt fact.** With CMK, Key Vault is a hard dependency. If the server can't reach the key, it transitions to **Inaccessible** within ~60 minutes and denies all connections. Recovery: within ~60 minutes after the key becomes reachable again.

**Ways to break it (all real):**
- Key Vault deleted (recoverable via Key Vault soft-delete; relies on us having soft-delete + purge protection on, which the doc strongly recommends).
- Key disabled, deleted, or expired.
- RBAC role / access policy revoked from the managed identity.
- Managed identity itself deleted from Entra ID.
- Key Vault firewall rules tightened so the server can't reach it.

**Operational mitigations the doc calls out:**
- Resource lock on Key Vault (prevents accidental delete).
- Soft-delete + purge protection on (mandatory recovery period).
- "Allow trusted Microsoft services to bypass this firewall" so the Postgres service can always reach Key Vault even with public access disabled.
- Activity log alerts on key-access failures, hooked into action groups for paging.
- Resource Health monitoring (DB shows Inaccessible after first denied connection).

**Implication.** With CMK, our Postgres availability SLO is the **product** of Postgres availability and Key Vault availability, plus operator error in the middle. This is the single biggest reason to ship on SMK and add CMK only when a customer pulls. The mitigations are real but they are nonzero ongoing operational load — locks, alerts, runbooks for "Key Vault is wedged" — and "lawyers are calling because we're down for an hour" is a much worse outcome at our stage than "Microsoft holds our keys."

### 5. pgcrypto on Flex Server

**Sources:** MS Learn — [Allow extensions](https://learn.microsoft.com/en-us/azure/postgresql/extensions/how-to-allow-extensions); MS Learn — [Considerations with the Use of Extensions](https://learn.microsoft.com/en-us/azure/postgresql/extensions/concepts-extensions-considerations); Microsoft Tech Community — [Secure sensitive data with pgcrypto extension in Azure PostgreSQL Flexible Server](https://techcommunity.microsoft.com/blog/adforpostgresql/secure-sensitive-data-with-pgcrypto-extension-in-azure-postgresql-flexible-serve/3705870).

**Availability.** pgcrypto is supported on Flex Server. It is on the extension allowlist and we enable it by adding it to the `azure.extensions` server parameter and then `CREATE EXTENSION pgcrypto`. No license, no ticket.

**OpenSSL 3.0 / Azure Linux 3.0 caveat.** Servers on Azure Linux 3.0 use OpenSSL 3.0, which moves several legacy algorithms (DES, 3DES, Blowfish, MD5 in some uses) into a non-default "legacy provider" that is **not loaded** for pgcrypto. AES-256 (CBC and GCM), SHA-2 family, and modern HMAC are unaffected. Microsoft's own community blog post demonstrating pgcrypto uses `gen_salt('bf')` (Blowfish) for password hashing — that example may not work on current Flex servers and is the wrong pattern for our use case anyway.

**Bigger architectural question: pgcrypto vs application-layer envelope encryption.** pgcrypto encrypts inside the database, which means:

- The encryption key must reach the database, typically as a query parameter or `SET` value. The plaintext key lives in connection memory and in any query logs that aren't carefully scrubbed.
- Backups taken at the storage layer contain ciphertext (good), but anyone with a working DB connection and the key can decrypt at will (same as plaintext from their perspective).
- Indexing encrypted columns requires deterministic modes, which leak frequency information. For our case we don't need to index encrypted blobs, so this is moot.

The alternative — **envelope encryption in Go** — does the AES-GCM operation in the API/worker, talking to Key Vault for the key-encrypting key, and stores ciphertext + IV + key reference in the DB. Postgres never sees the plaintext or the data key. This is cleaner for our threat model: a compromised DB connection or a stolen backup blob both give the attacker only ciphertext, and the keys live in Key Vault under our IAM.

**Recommendation.** Skip pgcrypto for the column-level encryption work. Do envelope encryption in Go using Azure Key Vault as the key-encrypting-key store. pgcrypto stays available if we ever want it for narrower jobs (e.g., one-way hashing of a lookup token) but it shouldn't carry the contract-text protection.

**Implication for #4 (Key Vault outage).** Envelope encryption shifts the Key Vault dependency from the database server to the application. Tradeoff: API workers can cache unwrapped data keys in memory for the duration of a Review, so a Key Vault blip doesn't take down in-flight work the same way it nukes a CMK-protected DB. We get a softer failure mode in exchange for application-layer code we have to write.

### 6. TLS posture

**Sources:** MS Learn — [Transport Layer Security (TLS) in Azure Database for PostgreSQL](https://learn.microsoft.com/en-us/azure/postgresql/security/security-tls); MS Learn — [Networking overview using SSL and TLS (Flex)](https://learn.microsoft.com/en-us/azure/postgresql/flexible-server/concepts-networking-ssl-tls).

**Default.** All client connections to Flex Server require TLS. TLS 1.0 and TLS 1.1 are denied. TLS 1.2 is the floor by default; TLS 1.3 is supported.

**Tightening.** Set the `ssl_min_protocol_version` server parameter to `TLSv1.3` to require 1.3 across the board. Go's `pgx` driver and modern Postgres clients all speak 1.3, so this is a costless upgrade for our stack.

**Recommendation.** Set `ssl_min_protocol_version = TLSv1.3` on the prod server from day one. Cost zero, marginal but real defense-in-depth, plus a cleaner answer on questionnaires than "TLS 1.2+." Leave staging looser if we ever need to test from an older client.

**Operational note (2026 cert rotation).** DigiCert Global Root CA intermediate CA rotation is in flight Q1 2026. Anyone pinning intermediate certs (uncommon but possible) needs to handle the chain change. Standard root-store-based clients are fine. Worth one line in the runbook.

### 7. Competitor disclosures

**Sources:** [Ironclad Per-Tenant Encryption Overview](https://support.ironcladapp.com/hc/en-us/articles/22390629369623-Per-Tenant-Encryption-Overview); [Harvey Security](https://www.harvey.ai/security); [Spellbook Security](https://www.spellbook.legal/security).

| Vendor | Default at-rest | Default in-transit | BYOK / per-tenant keys | SOC 2 | Notable scope notes |
|--------|-----------------|--------------------|------------------------|-------|---------------------|
| Ironclad | AES-256 | TLS 1.2+ | "Per-Tenant Encryption" / HYOK is a paid Security & Data Pro add-on. Customer-supplied root key in their own cloud. | Yes (Type II implied via SafeBase portal) | HYOK only encrypts contract documents (.docx, .pdf). Not metadata, findings, or user-generated content. |
| Harvey | AES-256 | TLS 1.2+ | Not publicly advertised as a customer-facing feature. Isolated processing environments per workspace. | SOC 2 Type II + ISO 27001, audited annually by Schellman; pen tests by NCC and Bishop Fox. | Contractual no-train commitment for inputs/outputs/uploads. |
| Spellbook | "Encryption at rest and in transit" — no algorithm published | Industry-standard, no version published | Not advertised. | SOC 2 Type II, GDPR, CCPA. | ZDR negotiated with OpenAI and Anthropic. Hosted in Canada and US. |

**Reading the floor.** The market floor for our customer set is:
- AES-256 at rest, TLS 1.2+ in transit
- SOC 2 Type II (not Type I — that's a milestone, not a destination)
- No-training contractual commitment with the LLM provider
- ZDR with the LLM provider

Our planned posture (Flex SMK + TLS 1.3 + SOC 2 Type I in flight + Anthropic ZDR via Azure Foundry) **clears the floor for transit and at-rest**. The gap on the questionnaire is "Type II in flight, not yet complete" — same as everyone else at our stage.

**Reading the ceiling.** Per-tenant / customer-key encryption is consistently sold as an enterprise upsell, even at Ironclad which has the most public detail about it, and even at Ironclad it's scoped narrowly to uploaded contract files — not metadata, not findings. This validates two of our scoping decisions:
- #3 (no BYOK at launch) — comparators don't ship it at the entry tier either.
- #5 (column-level envelope encryption for contract text + Findings JSONB) — if we ship this in the base product, our default scope is **broader** than Ironclad's enterprise add-on. Worth saying explicitly on a questionnaire and in marketing.

**Caveat to flag.** Three vendors in three different shapes: Ironclad publishes the most detail, Harvey publishes a polished compliance story, Spellbook is the most opaque on technical specifics. Don't anchor on Spellbook's vagueness — they probably do something fine, they just don't disclose. Pricing and customer mix differ enough across the three that "what they ship" is a loose floor, not a spec.

### 8. Published patterns / pitfalls
_(pending — lower priority. Will revisit if a specific question surfaces.)_

## Recommended v1 posture (synthesis)

Pulling the findings together into the actual configuration we'd ship:

1. **Flex Server with service-managed keys (SMK).** AES-256 at the storage layer, no Key Vault dependency, no additional ops surface. Clears the at-rest questionnaire bar.
2. **`ssl_min_protocol_version = TLSv1.3` on the prod server.** Free tightening from the TLS 1.2 default. Staging stays at default for client-compat testing.
3. **Application-layer envelope encryption (Go) for contract text, supplemental docs, `Finding.details` JSONB, user-provided answers, Prefix.** AES-256-GCM with a per-Org data-encryption key, wrapped by a key-encrypting key in Azure Key Vault (Standard tier). Skip pgcrypto for this — wrong layer for our threat model.
4. **Indexable scalars stay plaintext.** Finding type, severity, Lens id, timestamps, FK ids, status. No queryable-while-encrypted; no deterministic-encryption tarpit.
5. **Key Vault hardening from day one** even before we adopt CMK at the storage layer, because envelope-encryption KEKs already live there: soft-delete on (90 days), purge protection on, resource lock, RBAC permission model, diagnostic logs to Log Analytics.
6. **Defer CMK at the storage layer.** Add when (a) a customer asks, or (b) we move into AmLaw / regulated in-house. Migration path is PITR-restore into a CMK-configured server, which is annoying but not a data-model migration.
7. **Defer Managed HSM / FIPS 140-3.** Standard Key Vault until a customer questionnaire requires HSM-backed keys.

**What this story sounds like on a security questionnaire:**
- "All data encrypted at rest with AES-256 (Azure platform)."
- "All connections require TLS 1.3."
- "Sensitive contract content and analytical findings additionally encrypted at the application layer with AES-256-GCM using per-Organization keys, wrapped by a key-encrypting key managed in Azure Key Vault."
- "Customer-managed root keys (BYOK) available on the enterprise tier (roadmap)."

That answer is **at or above** what Ironclad / Harvey / Spellbook publish at the entry tier, with the bonus that the column-scope is broader than Ironclad's enterprise HYOK.

**Threat-model coverage check:**
- *Lost backup blob:* Storage-layer AES-256 protects the blob; envelope-encrypted columns stay encrypted even if SMK is somehow defeated. **Covered.**
- *Rogue Azure operator:* Storage-layer key is Microsoft-held (SMK), so storage-layer alone doesn't defend. Envelope encryption with our Key Vault-held KEK does — Microsoft doesn't have the KEK. **Covered, but only if envelope encryption ships as planned.** This is the single most important load-bearing implementation item.

## Open questions and contradictions

- **Burstable + CMK confirmation.** The data-encryption doc doesn't gate CMK by SKU on Flex, but we should portal-verify before publishing any spec text that says "CMK is available on Burstable." Evidence is permissive, not affirmative.
- **Threat-model honesty.** Don't ship the "rogue Azure operator" story on a questionnaire until envelope encryption is actually live in code. SMK alone does not deliver it.
- **#8 not yet done.** Search for published war-stories of legal/contract-AI vendors on Azure Postgres specifically. Lower priority — only matters if we hit a non-obvious failure mode.
- **OpenSSL 3.0 algorithm verification.** Confirm AES-256-GCM is fully available (it is in default OpenSSL 3 builds), and confirm we're not relying on any pgcrypto algorithm that ended up in the legacy provider. Light task once we have a server provisioned.
- **`pg_dump` and logical backups.** This research is scoped to platform encryption. If we ever do logical exports (e.g., for migrations or analytics), envelope-encrypted columns export as ciphertext — fine — but key-encrypting-keys must travel with the operator, not the dump. Worth a runbook line, not a research item.

## Decisions log

- **2026-04-30 — Ship v1 on Flex Server with SMK, not CMK.** Reason: CMK adds a Key Vault outage dependency (server transitions to Inaccessible within ~60 min if Key Vault is unreachable) and operational ops cost (per-region vaults and identities, manual or auto rotation, etc.) that solo-practitioner customers don't need on day one. Migration to CMK later is a PITR-restore, not a data migration.
- **2026-04-30 — Use application-layer envelope encryption in Go (not pgcrypto) for column-level encryption.** Reason: pgcrypto requires the data key to enter the database, which puts plaintext key material in connection memory and query logs. Envelope encryption keeps plaintext keys in the application process and ciphertext in the DB; better fit for the rogue-operator threat-model leg.
- **2026-04-30 — Defer application-layer envelope encryption out of v1 entirely.** Reason: dev cost and operational shakedown burden too high for a pre-funding launch. SOC 2 Type I and Type II do not require it; SMK + TLS 1.3 + access controls clear the audit floor. The "rogue Azure operator" threat-model leg is dropped from v1 and added back when envelope encryption ships. Trigger to add: a firm customer pulling for cryptographic tenant isolation, or pre-Type-II posture for enterprise legal sales. Designed-for-future obligation: when we model new tables, leave room to convert sensitive columns to `bytea` without a painful schema rewrite.
- **2026-04-30 — Set `ssl_min_protocol_version = TLSv1.3` on prod from day one.** Reason: free tightening, all our intended clients support 1.3, cleaner questionnaire answer.
- **2026-04-30 — Defer Managed HSM / FIPS 140-3.** Reason: Standard Key Vault meets the Type I/II floor and what comparator vendors publish; HSM-backed keys are an enterprise-tier upsell pattern, not a launch requirement.

## Decisions log

_(empty — capture each decision with date, the question it answers, and the one-line reason)_
