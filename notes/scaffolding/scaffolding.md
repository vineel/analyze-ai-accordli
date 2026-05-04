# Overview

The core product that we will call "Accordli Analyze" is still in product definition with the Product Team. Meanwhile, our window to ship product seems to be closing. Microsoft just came and ate part of our product roadmap. We need to ship the product ASAP after the product is finally defined.

Our strategy is to build the "productization" scaffolding that *any similar* product would need.

# Scaffolding
We are going to build around a "starter" application, that is so simple that it is trivial, but at least kind of the same shape as our final app. When the product definition is finalized, we will "swap out" the starter app for the real app.

## The Starter App
From the user's perspective:
1. Main Page allows the user to see previous "Matters".
2. Click Plus to add a "Matter" in the database. 
3. The user can uploads a .DOCX file.
4. The file is saved, converted to markdown, which is saved in the database against the Matter.
5. The document is summarized by LLM call, and the summary is saved to the database.
6. The Matter Page shows the summary and the markdown content. There is a button to download the original file.

### Subsystems and Integrations
#### Hosting
Azure is our target cloud. We will use Azure services unless we have a good reason to choose another.

#### Identity
The main integration will be with WorkOS.
* Authentication
* login
* signup
* password reset
* SSO/SAML 
* Invites and Member Admin

#### Billing and Charging
The main integration will be with Stripe.
* Credit Card entry and storage
* Recurring Billing
* Extra one-time billing
* Plan-backed Credit Ledger
* Net 30 Invoicing and Billing (future phase)
* Tax calculation and collection will be Stripe Tax

#### Core App Plumbing
* API will be built in go
* Database will be Azure Database for PostgreSQL
* Database backup using built-in service
* LLM calling will happen via a Queue (River + Postgres)
* File Storage will be Azure Blob
* File Storage backup using Versioning
* Frontend will be a vite + React web app
* In-app and email
* Document Parsing & Conversion (docx -> markdown using pandoc and go custom processing)

#### Devops and Deployment
* Source Control in Github
* Development on local machines
* Staging in Azure (triggered by Github)
* Production in Azure (manually triggered)
* API Container (perhaps N instances, load balanced)
* Worker Container (perhaps 2 instances for redundancy)
* Admin Container (Administration, Authoring, and Customer Service apps)
* Rate Limiting using Cloudflare(edge)
* Search (in-product) using Postgres full text search
* PDF/report generation

#### Observability and Analytics
* TBD

#### Customer Support Functions
* Soft Delete
* Data Export

#### LLM
* The vast majority of prompts will be serviced by Claude Sonnet (latest version)
* Primary vendor is Azure, which hosts Anthropic models.
* Secondary (fallback) vendor is Anthropic.
* LLM observability using Helicone (full logging for dev/staging, metadata-only for Prod)

#### Email Provider
* Postmark

#### Other Aspects
* SOC2 readiness from Day 1
* Will build toward certification
* Encryption at rest, in transit
* Secrets management in Azure Key Vault
* Backup (what's Azure standard here?)
* Will need to be able to "completely erase" most data ala CCPA


Posthog for analytics and feature flags?
