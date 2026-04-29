# Platform Overview

Accordli is a business-to-business legal technology platform designed for use by lawyers and legal professionals across multiple customer segments, including:

- Independent practitioners
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
- A corporate legal department
- A business entity purchasing access for legal or contracting personnel

An Organization is required for all accounts.

### Department

A **Department** represents an optional subdivision within an Organization.

Departments may be used to group users by legal team, practice area, business unit, office, client group, or other internal structure.

An Organization may have zero or more Departments.

### User

A **User** represents an individual human authorized to access the Platform.

Each User must belong to exactly one Organization. A User may also belong to one Department within that Organization, where departmental grouping is enabled or applicable.

Initial model:

```text
Organization
    Department (optional)
        User
```
or
```text
Organization
    User
```

