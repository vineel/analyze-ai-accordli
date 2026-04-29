# Platform Overview

Accordli is a business-to-business legal technology platform designed for use by lawyers and legal professionals across multiple customer segments, including:

- Solo practitioners
- In-house legal departments
- Law firms

The Platform must support a range of customer profiles, from solo users to large organizations with multiple legal teams, departments, administrators, and billing arrangements.

---

# User and Account Structure

Because Accordli serves both individual practitioners and larger institutional customers, the Platform must support a flexible user management model capable of accommodating both simple and complex organizational structures.

## Core Account Hierarchy

### Organization

An **Organization** represents the primary customer account within Accordli. Every user must belong to exactly one Organization.

An Organization may represent, for example:

- A solo legal practitioner
- A law firm
- An in-house legal department
- An enterprise customer

An Organization is required for all accounts.

### Department

A **Department** represents a subdivision within an Organization.

Departments may be used to group users by legal department, practice area, business unit, office, client group, or other internal structure.

An Organization may have 1 or more Departments.

### User

A **User** represents an individual human authorized to access the Platform.

Each User must belong to exactly one Organization and one Department within that Organization. 

(A solo practitioner can have a default Org and Department, which are not evident in the normal UX flow.)

Initial model:

```text
Organization
  -> Department
     -> User
```


---

# Pricing Model

Accordli’s core functionality requires significant usage of AI models and related computational resources. As a result, the Platform’s pricing model must be usage-sensitive.

At the same time, usage must be presented to customers in a manner that is understandable, predictable, and commercially intuitive. Customers should be able to estimate their expected monthly cost and manage their usage without needing to understand token consumption, model costs, or other technical measures.

To accomplish this, Accordli will price usage primarily by **Agreement Review Credits**, allocated by subscription tier. 

An **Agreement Review Credit** represents the analysis of one contract or agreement through the Platform’s supported review workflows. Reports, memoranda, summaries, and related outputs generated from an analyzed contract may be included without additional per-output charges, subject to applicable fair-use or abuse-prevention limits.

---

# Subscription Tiers

## Solo Practitioner Plans

#### Pro Plan

- **Monthly Fee:** $200 per month
- **Included Usage:** 10 Agreement Review Credits per billing month
- **Included Outputs:** Unlimited reports and memoranda for analyzed contracts

#### Gold Plan

- **Monthly Fee:** $400 per month
- **Included Usage:** 25 Agreement Review Credits per billing month
- **Included Outputs:** Unlimited reports and memoranda for analyzed contracts


#### Extra Agreement Review Credits

- **Price:** $100
- **Included Usage:** 10 additional Agreement Review Credits
- **Expiration:** 12 months from date of purchase

Unused Agreement Review Credits in an Extra Contract Pack expire one year after purchase and are not refundable except as otherwise required by applicable law or expressly provided by Accordli.


#### Billing Terms

Customers must provide and maintain a valid payment method, such as a credit card, PayPal or other supported payment method.

Subscriptions are billed monthly in advance. The customer’s billing date is based on the date on which the subscription is activated, unless otherwise specified in the customer’s order form or applicable agreement.

If a customer cancels a subscription, the account remains active through the end of the then-current billing period. Cancellation does not entitle the customer to a prorated refund unless expressly provided by Accordli or required by applicable law.

#### Refund Policy

A customer may be eligible for an automatic refund during the first seven days of a billing period, provided that the customer has analyzed no more than two contracts during that billing period.

Refund eligibility may be subject to additional abuse-prevention rules and may be modified for enterprise or custom arrangements.

---

## Team Plans

Team plans are designed for law firms, in-house legal departments, and other multi-user organizations.

### Small Team Plan

- **Monthly Fee:** $600 per month
- **Included Seats:** 3 users
- **Included Usage:** 40 Agreement Review Credits per billing period, shared across the Organization
- **Included Outputs:** Unlimited reports and memoranda for analyzed contracts
- **Included Features:** Team Dashboard

### Large Team Plan

- **Monthly Fee:** $2,000 per month
- **Included Seats:** 10 users
- **Included Usage:** 130 Agreement Review Credits per billing period, shared across the Organization
- **Included Outputs:** Unlimited reports and memoranda for analyzed contracts
- **Included Features:** Team Dashboard

---

## Enterprise Plan

Enterprise plans are available for customers requiring custom commercial, administrative, security, or operational terms.

Enterprise pricing and usage limits will be determined on a case-by-case basis and may include a custom combination of:

- Number of seats
- Agreement Review Credits
- Departmental or matter-level administration
- Team Dashboard access
- Custom reporting
- Security, compliance, or procurement requirements
- Dedicated support
- Custom billing terms

Enterprise customers must contact Accordli for pricing and onboarding.

---

# Content Ownership

Matters are owned at the Department level. Organization administrators may view and administer all Matters within the Organization.