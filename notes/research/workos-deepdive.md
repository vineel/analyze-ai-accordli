# WorkOS Deep Dive — Research Log

Investigation into whether WorkOS is the right identity / authorization
substrate for Accordli analyze. The brief: solo → small team → large team →
enterprise customers, security-forward at launch, SOC 2 Type I ~month 9, lawyer
audience, two-engineer team. Locked-ish in current notes; nothing committed.

This file is the working log. Findings, open questions, and the eventual
recommendation all live here. Updated as the investigation proceeds.

---

## Phase 1 — Concerns and Questions Driving the Investigation

These are the questions whose answers will determine whether WorkOS is a
"yes, default to this" or a "yes, but with these caveats" or "no, here's why
not." Grouped by axis.

### A. Fit with our identity / data model

1. **Organization / Department / User mapping.** WorkOS has Organization and
   User as first-class concepts. Accordli has Org → Department → User. Does
   WorkOS represent Department natively? If not, do we model Department in our
   own DB and use WorkOS only for Org+User, or use WorkOS Groups / RBAC roles
   to express Department, or use FGA?
2. **Solo practitioner ergonomics.** Solo signups need a dead-simple flow
   (email + password, or Google). Does AuthKit's hosted UX gracefully hide
   "select organization" / "join workspace" steps for a single-user account?
   Can the same Org row work invisibly for solos and visibly for firms?
3. **Cross-Org user identity.** Lawyers move firms; one human may end up with
   multiple Org memberships over time. Does WorkOS support a single User
   across multiple Organizations, or is User scoped per Org? (This affects
   data export and audit trails.)
   
   Tom: We don't care about this right now.
   
4. **Mid-flight tenancy promotion.** A solo on a Pro plan invites a partner;
   the account becomes a small team. Does the model support that without
   forcing a re-signup, or do we need to design a "promote Org" path?
   
   Tom: Yes we do need to design a "promote org" path.

### B. Surface area: what we get vs. what we still build

5. **AuthKit hosted UI vs. embedded SDK.** What's the default? Hosted UI
   (workos-managed login pages) vs. headless / embedded React components.
   Implication for our design system, branding, and lawyer-grade trust signals.
   
   Tom: We prefer React components for most things where it makes sense.
   
6. **MFA, password reset, magic link, social, passkeys.** Which are included
   in AuthKit out of the box, which are paid add-ons, which require config
   work from us?
7. **Session management.** Token format, refresh, revocation, "log out
   everywhere," device list. What's API-accessible vs. only in their UI?
8. **Invite flow.** Org-admin invites a teammate. Does WorkOS handle the
   email, the accept-link, the join-org-on-signup join? Or do we send the
   email and call WorkOS APIs to mint memberships?
9. **Admin Portal.** Does this give the customer's IT admin a useful self-
   serve experience for SSO and SCIM, or is it a pretty wrapper that we
   still have to integrate against?
   
   Tom: Let's understand what a sample flow would be to integrate an SSO customer.
   
10. **RBAC vs. FGA.** When does the line cross from "roles are enough" (Owner,
    Admin, Member) to "we need fine-grained authorization"? Department-scoped
    Matter visibility looks like an FGA-shaped problem; or does it stay simple
    with org_id + dept_id checks in our own code?

### C. Audit logging boundary

11. **What WorkOS Audit Logs capture by default.** Identity events presumably:
    sign-in, MFA enrollment, SSO config change, SCIM provisioning. Do they
    capture _our_ application events (Matter created, Review run, finding
    exported), or are those strictly our responsibility?
12. **Where the boundary should live.** Two-table model (`audit_events` in our
    DB + WorkOS Audit Logs) vs. unified (everything funneled to WorkOS) vs.
    inverted (everything in our DB, WorkOS data mirrored in). Each has
    consequences for SOC 2 evidence, for customer-facing audit-log UI, and
    for log streaming to enterprise SIEMs.
13. **SIEM streaming.** Enterprise customers will want logs streamed to their
    Splunk / Datadog / Sentinel. Does WorkOS's "$125/mo per SIEM connection"
    cover only WorkOS-recorded events, or can we send our own events through
    that pipeline too?
14. **Retention and pricing of audit log volume.** $99/mo per 1M events at
    today's pricing. What's our event rate likely to be? At what customer
    scale does this become a meaningful line item?

### D. SOC 2 and compliance posture

15. **What WorkOS can show us.** SOC 2 Type II, ISO 27001, GDPR, HIPAA BAA
    availability. Do we get their reports under NDA without a sales call?
16. **Subprocessor implications.** Adding WorkOS adds a subprocessor on the
    list lawyers will read. Is the brand recognized enough that this is a
    plus, or does it introduce friction with regulated/in-house customers
    who prefer Okta / Entra?
17. **Data residency.** Where does WorkOS host identity data? US only? EU
    region available? Relevant for any future EU customer.
18. **DPA, BAA, MSA.** Can we sign these without a sales motion at our scale?
    What's the smallest plan that includes them?

### E. Pricing and commercial fit

19. **AuthKit's "first 1M MAUs free" claim.** Real, durable, or marketing?
    What turns from free into paid first — extra connections, audit log
    events, Admin Portal, custom domain?
20. **Per-connection SSO/SCIM cost.** $125 per connection per month for the
    first 15. At what plan tier do we charge customers for SSO (the classic
    "SSO tax")? Does the unit economics of Small Team / Large Team plans
    cover this if 30% of those orgs ask for SSO?
21. **Custom domain $99/mo.** Do we need this on day one for our lawyer-grade
    brand, or can we live on `auth.accordli.com` via DNS without paying?
22. **Hidden line items.** Vault, FGA, Radar, MFA — what's metered, what's
    flat, what's gated to enterprise tiers.
23. **Annual vs monthly, minimums, startup program.** Does WorkOS have a
    YC / startup deal that materially changes the math?

### F. Risk / lock-in / vendor health

24. **Migration off.** If we wanted to leave in year 3, can we export users,
    org memberships, hashed passwords, MFA factors, audit logs? Which of
    those _can't_ be migrated and would force a forced password reset?
25. **Long-term vendor health.** WorkOS is well-funded and growing as of
    early 2026, but identity is a long-tail commitment. What's the Plan B
    if they get acquired and the product changes character (cf. Auth0/Okta)?
26. **Outage / failure mode.** WorkOS is in the hot path of every login.
    Their uptime history, incident communication, and our fallback if
    AuthKit is down for hours. (Probably no fallback; people just can't log
    in. Confirm if that's acceptable.)
27. **Practical limitations users have hit.** What do experienced WorkOS
    customers complain about on HN / Reddit / blogs? Particularly around
    hosted UI flexibility, multi-org users, and the AuthKit → enterprise SSO
    transition.

### G. Build vs. buy decision frame

28. **What would we build if we self-hosted (Ory / Zitadel)?** Cost in
    engineer-weeks for: AuthKit-equivalent UX, Org/Member CRUD, SSO ingest,
    SCIM consumer, audit log UI, Admin Portal, MFA, magic link, password
    reset. Compare to WorkOS price at expected scale.
29. **What does Clerk give us that WorkOS doesn't, and vice versa?** Clerk's
    React UX is regularly cited as best-in-class; their B2B / enterprise
    story is weaker. For lawyers buying us, which matters more?
30. **Reversibility window.** Given that auth is sticky, when is the latest
    we could switch off WorkOS without re-signing every user? (Partly
    determined by #24.)

### H. Integration with Stripe

31. **What does a signup flow look like** that includes choosing a plan and paying for it?
32. **Cancelation** When a user cancels an account, how is that handled?

### I. Integration with our Accordli app

33. How does login work?
34. How does signup work?
35. How does signout work?
36. How does cancelation/termination work?

---

## Phase 2 — WorkOS Product Surface (Findings)

_(populated as research proceeds)_

### Major product areas (snapshot)

| Product | One-line | Notes |
|---|---|---|
| AuthKit | Hosted authentication: email/password, social, magic auth, MFA, sessions | The default frontend on top of User Management |
| User Management | Users, organizations, memberships, invites, password & email primitives | The data layer AuthKit sits on |
| SSO | SAML / OIDC connectors per customer org | Per-connection pricing |
| Directory Sync | SCIM (and HRIS) consumer for user lifecycle | Per-connection pricing |
| Admin Portal | Self-serve UI for customer IT admins to configure their SSO/SCIM | Branded, link-in flow |
| Audit Logs | Ingest + retain + stream identity and app events | $/event + $/SIEM connection |
| RBAC | Roles + permissions, org-scoped | |
| FGA | Fine-grained auth (Zanzibar-style) | Separate product |
| Vault | Encrypted KV / secret storage | |
| Radar | Bot / fraud / abuse signals on login | |
| Pipes | Customer connects 3rd-party accounts | |
| Feature Flags | Flag delivery | |
| Widgets | Pre-built UI for common enterprise flows | |
| Domain Verification | Customer claims a domain | Used to gate SSO defaults |

### Pricing snapshot (as of May 2026 — confirm before quoting)

| Item | Price |
|---|---|
| AuthKit MAU | First 1M free; $2,500/mo per additional 1M |
| SSO (per connection per month) | $125 → $100 → $80 → $65 → $50 sliding by volume |
| Directory Sync (per connection per month) | Same scale as SSO |
| Audit Logs — SIEM streaming | $125/mo per connection |
| Audit Logs — event retention | $99/mo per 1M events |
| Radar | First 1k checks free; $100/mo per 50k |
| Custom domain | $99/mo |
| Admin Portal / Vault / FGA / MFA | Pricing not on public page; pull from sales or current docs |

> Marketing claim: "Free to start, you only pay for what you use." Validate
> that no minimum kicks in at the AuthKit free tier and that adding the first
> SSO connection doesn't force a different plan.

---

## Phase 3 — Mapping to Accordli's Model

_(populated as research proceeds)_

## Phase 4 — Pricing Projections

_(populated as research proceeds)_

## Phase 5 — Compliance / SOC 2 Posture

_(populated as research proceeds)_

## Phase 6 — Audit Log Boundary

_(populated as research proceeds)_

## Phase 7 — Risks and Lock-in

_(populated as research proceeds)_

## Phase 8 — Recommendation

_(populated last)_
