Things to Research & Open Questions

1. encryption at the postgres level for azure.
2. client-based encryption -- how does this work across blob storage and postgres?
3. do we use lago free for keeping track of credits and ledger? or orb? (see docs)
4. what kind of dashboards, consoles, etc. will we need to operate the whole app? what will be spread across diff products like payments, billing, etc. and what will be handrolled by us?
5. what do we actually have access to, under SOC2-ready security guidelines?
6. how do we build a dev system around this that is azure-compatible but has good dx?
7. Account Page - must be designed
8. Purchase More Credits flow
9. Sign Up flow (with plan choice and payment)
10. Department-vs-Organization billing tensions in Team plans. Two related sub-questions, same root cause (Org owns the budget; Departments own the work):
    a. **Matter ownership / cross-Department sharing.** Shared org-wide ARC pool + Department-owned Matters interact awkwardly. One Department can burn the firm's pool while another starves; visibility and contributor access across Departments is undesigned.
    b. **Cardholder / budget divergence within one Org.** Today: one Stripe Customer per Org → one card → one shared ARC pool. Two Departments in a firm with separate budgets / separate cards do not fit this. Workaround in MVP: they sign up as two separate Organizations (the "same firm" relationship is invisible to Accordli). Real enterprise pattern (one MSA, multiple billing groups under it) deferred until an Enterprise customer asks; would require a `BillingAccount` level between Org and Department mapping 1:1 to Stripe Customer. See `notes/claude-code-artifacts/stripe-research-scratch.md` Q14.

    Possible MVP-compatible mitigations for (a) without restructuring billing: per-Department ARC quotas (admin-set caps within the shared pool), or explicit "Team plans require a single shared budget; orgs that can't agree should have separate Orgs."
11. Matter lock + retry UX flow. Today the spec locks Matters once a ReviewRun starts; need a designed flow for "I want to upload an amendment after I see the findings."
12. Solo practitioner Org/Dept — concrete data model. Are the rows literally there with hidden UX, or is there a special case in the access-control code?
13. Refund policy revision. Current wording allows the refund window to recur monthly. Likely revision: one-time per email address, possibly solo-only, possibly support-intervention only.
14. Verify Azure Foundry's `cache_control` semantics match the direct Anthropic API. The whole prefix-caching design and per-ARC cost math depend on this.
15. WorkOS deep dive. What we get out of the box (AuthKit, Organizations, SSO/SCIM, audit logs) vs. what we have to build. Where the audit-log boundary lives between WorkOS-recorded events and our own `audit_events` table. Likely deserves its own `notes/research/workos-deepdive.md`.
