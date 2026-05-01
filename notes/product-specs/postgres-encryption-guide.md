# Postgres Encryption Guide — Accordli on Azure

The corresponding Postgres deployment is Azure Database for PostgreSQL Flexible Server, Burstable B2s at starter scale, in East US 2 (per the platform stack).

## Concepts

### Encryption at rest vs in transit

**At rest** means data sitting on disk. **In transit** means data moving between two processes — client to database, application to Key Vault, primary to replica. Both need encryption, but they use different mechanisms (block-cipher modes for at-rest, TLS for in-transit) and fail in different ways.

### The KEK / DEK envelope pattern

Almost all production-grade encryption uses two keys, not one:

- A **data-encrypting key (DEK)** does the bulk AES work on the actual plaintext. Big jobs use AES-GCM or AES-CTR.
- A **key-encrypting key (KEK)** encrypts the DEK. The KEK is stored separately, in a hardened key store. The DEK travels with the ciphertext as a wrapped blob.

This is "envelope encryption." Why two keys: rotating a KEK is cheap (re-wrap the DEKs), but rotating a DEK requires re-encrypting all the data. So you keep DEKs as long as the data lives and rotate KEKs whenever you want.

### Storage layer vs application layer

Encryption can live at two layers, and they are independent.

- **Storage-layer encryption** is performed by the database/disk infrastructure. Plaintext enters Postgres normally; the storage subsystem encrypts before writing to disk. This protects backup blobs and physical media. It does **not** protect against anyone with a working DB connection — they read plaintext as usual.
- **Application-layer encryption** is performed by our Go code. The application encrypts data before sending it to Postgres, and decrypts after reading it. Postgres only ever sees ciphertext for those columns. This protects against an attacker with valid database access (stolen credentials, a misused service principal, a developer with too-broad read rights) — even a legitimate `SELECT` returns ciphertext, and decryption requires separate access to the KEK in Key Vault. It also protects against an exfiltrated row dump for the same reason.

Most production systems run both. **We run only storage-layer encryption at v1.** Application-layer envelope encryption is on the deferred roadmap; the concept is documented here because we'll add it when a customer pulls it in.

### Tenant isolation

Tenant isolation answers: "if Org B's data leaks (or Org B is malicious), can they read Org A's data?" There are two flavors:

- **Logical isolation:** rows are scoped by `org_id` and queries enforce it. Standard, necessary, not sufficient on its own for a "cryptographic isolation" claim.
- **Cryptographic isolation:** Org A's data and Org B's data are encrypted under *different keys*. A process holding Org B's key cannot decrypt Org A's data, full stop. This requires per-Org keys at the application layer; storage-layer encryption (which uses one key for the whole database) does not deliver it.

Storage-layer encryption (SMK or CMK) and tenant isolation are **independent features**. They stack; we choose them separately.

## Terminology

| Term | Meaning |
|------|---------|
| **SMK** — service-managed key | Encryption key for the database, held by Microsoft. Default for Azure Postgres Flex. |
| **CMK** — customer-managed key | Encryption key for the database, held by *us* in our Azure Key Vault. Microsoft can't decrypt without it. The "customer" here is Azure's customer (Accordli), not the law firm. |
| **HYOK / BYOK** — hold-your-own-key / bring-your-own-key | Encryption key held by the *end customer* (the law firm) in their own cloud. Enterprise-tier upsell pattern. Not a default offering anywhere. |
| **DEK** — data-encrypting key | Symmetric key (AES) used to encrypt actual data. Travels wrapped, alongside the ciphertext. |
| **KEK** — key-encrypting key | Asymmetric or symmetric key used to wrap DEKs. Lives in Key Vault. Never leaves. |
| **Per-Org DEK** | A DEK scoped to one Organization. Different Org, different DEK. Enables cryptographic tenant isolation. |
| **Envelope encryption** | The pattern of encrypting data with a DEK, then encrypting the DEK with a KEK. |
| **Azure Key Vault** | Azure's hardened key-management service. Standard tier is software-protected; Managed HSM is FIPS 140-3 hardware-backed. We use Standard. |
| **pgcrypto** | Postgres extension for in-database crypto operations. We do not use it. If we ever do column-level encryption, it will be envelope encryption in Go — pgcrypto exposes plaintext keys in query parameters and connection memory. |
| **Inaccessible** (server state) | Azure Flex Server state when it cannot reach its CMK. The server denies all connections. |

## Threat model

What v1 defends against:
- **Lost or exfiltrated backup blob.** Backup files (or their copies) end up somewhere unintended. Anyone reading them sees AES-256 ciphertext at the storage layer; the key is held by Microsoft.
- **Tampered or intercepted DB traffic.** All connections to Postgres require TLS 1.3.

What v1 does **not** defend against (be honest about this on questionnaires):
- **Rogue Azure operator.** Microsoft holds the storage-layer key. Our v1 posture trusts Microsoft with database content. If a customer requires that Microsoft cannot read our content, the path is application-layer envelope encryption (see roadmap) and/or CMK.
- **Compromised application process.** If our Go API is compromised, the attacker reads whatever the app reads.
- **Attacker with valid database credentials.** Stolen connection strings, a misused service principal, or a developer with too-broad read rights would see plaintext. v1 mitigates this with access controls, not encryption.
- **Subpoena resistance.** We can decrypt our own data; we can be compelled to. End-customer HYOK would change this.

## Spec — what we ship

In build order.

### 1. Storage-layer encryption: SMK on Flex Server

Provision Flex Server with the default service-managed key. AES-256 at the storage layer; encryption is automatic and applies to all databases, logs, WAL segments, and backups.

We do not configure CMK at v1. CMK adds a Key Vault availability dependency on the database (server transitions to Inaccessible within ~60 minutes of Key Vault becoming unreachable) and a per-region operational footprint we don't want to take on yet. SOC 2 Type I and Type II do not require CMK; SMK satisfies the at-rest control. Adding CMK later is a PITR-restore into a CMK-configured server, not a data migration.

### 2. In-transit encryption: TLS 1.3 floor

Set the server parameter `ssl_min_protocol_version = TLSv1.3` on the production server. Require TLS for all connections. Staging may run at the default TLS 1.2 floor for client compatibility testing.

Postgres clients (`pgx` for Go, `psycopg` for ad hoc admin work) speak TLS 1.3 natively; no client-side change required.

### 3. Access control as the v1 substitute for application-layer encryption

Application-layer envelope encryption is deferred. Until it ships, the protections that would normally come from app-layer crypto are carried by access control instead. Treat the following as required v1 controls, not nice-to-haves:

- **Least-privilege database roles.** The application connects with a role that has only the rights it needs. Read-only analytics roles are separate from read/write app roles. No human uses the app role for ad hoc queries.
- **No long-lived shared connection strings.** All Postgres credentials issued via Entra ID / managed identity where supported, or rotated short-lived credentials elsewhere.
- **Audit logging on the database.** `pgaudit` enabled for DDL, role changes, and reads of sensitive tables. Log shipping to Log Analytics with alerts on anomalous access volume.
- **No production data in lower environments.** Staging and dev never receive prod data dumps. If a repro is needed, anonymized fixtures only.
- **Backup access is RBAC-gated.** The storage account holding Flex Server backups has its own RBAC scope, separate from the app's identity. Restore permissions are held by a small named group.

These controls are what the SOC 2 auditor will look for in the absence of column-level encryption. They do not defend against a rogue Azure operator (only encryption-with-our-key does that), but they cover the realistic v1 threats.

### 4. What we explicitly do **not** do at v1

- **No application-layer envelope encryption.** Deferred to v2. Sensitive content sits in Postgres as ordinary types (`text`, `jsonb`, `bytea`), protected only by storage-layer SMK and database access controls.
- **No CMK at the storage layer.**
- **No Managed HSM.** Standard Key Vault used only for non-encryption secrets (DB connection strings, API keys, third-party credentials). Encryption-related Key Vault setup waits until v2.
- **No `pgcrypto` for column encryption.** When we add column encryption, it will be envelope encryption in Go — see the roadmap entry for the rationale.
- **No queryable-while-encrypted** (no deterministic encryption, no Always Encrypted equivalent).
- **No HYOK / customer-controlled root keys.**

## Operational rules

- **Backups.** Automatic backups and WAL segments are storage-encrypted by Flex Server. Logical exports (`pg_dump`) contain plaintext for sensitive columns at v1; treat dumps accordingly — they must not leave a controlled environment.
- **Logs and observability.** Application logs, traces, and Helicone payloads must not include contract content or analytical findings beyond what is necessary for an LLM call. Helicone sees prompts and responses (it has to, for caching) — that's the agreed-upon boundary. Internal logs stay scrubbed.
- **Credentials.** Postgres credentials and Key Vault access (for non-encryption secrets) are issued to managed identities, not service principals with stored secrets. No shared developer accounts on prod.
- **Restoring backups.** Restores happen into an isolated environment first, are validated, then promoted. The restore role is held by a named group, not the app identity.

## Roadmap items (deferred)

These are the future additions when a customer or compliance event pulls them in.

| Item | Trigger | Approximate cost to add |
|------|---------|-------------------------|
| **Application-layer envelope encryption** (per-Org DEKs, KEK in Key Vault, AES-256-GCM on contract text + Findings + user answers + Prefix) | A firm customer asks for cryptographic tenant isolation, or we want to publicly defend against the "rogue Azure operator" threat. Also the prerequisite for a credible Type II posture in front of an enterprise legal customer. | Meaningful: typed columns in Go, key-management code, key-rotation runbooks, re-encryption tooling for existing rows. Designing the table shapes for it now (so columns are `bytea`-friendly later) costs little; retrofitting after lots of data is in is the expensive part. |
| **CMK at storage layer** | Customer asks for "Microsoft cannot read our database storage" specifically | PITR-restore into CMK-configured server; new Key Vault key; ongoing ops cost from the Key Vault availability dependency |
| **Managed HSM** | Customer questionnaire requires FIPS 140-3 Level 3 | Move KEK from Standard Vault to Managed HSM; significantly higher unit cost |
| **HYOK / customer-controlled root keys** | Enterprise-tier feature for AmLaw / regulated in-house | Per-Org KEK held in customer cloud; cross-tenant Key Vault access pattern; meaningful ops + product investment |
| **Field-level deterministic encryption** | Need to range-query or join on encrypted fields | New library, careful key separation, frequency-leak risk; revisit only if an actual product feature requires it |

## Security questionnaire language

The two lines that summarize v1 for a customer security review:

> All data is encrypted at rest using AES-256, with platform-managed keys held by Microsoft in Azure Database for PostgreSQL Flexible Server. All client connections are required to use TLS 1.3.
>
> Application-layer encryption with customer-isolated keys, customer-managed keys (CMK), and customer-controlled root keys (HYOK) are on our roadmap.

What **not** to say at v1:
- Do not claim "Microsoft cannot read your data" — under SMK, the storage-layer key is Microsoft's.
- Do not claim "cryptographic tenant isolation" — v1 isolation is logical (`org_id` scoping), not cryptographic.
- Do not claim per-Organization data keys.

These claims become available once application-layer envelope encryption ships.
