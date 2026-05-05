# issues-1.pdf — Library Findings view

Source PDF: `issues-1.pdf` (5 pages)
Screen: **Library Findings** tab on a contract review detail page.

## Page chrome

- Back link
- Document header: `01_MobileDistribution_PixelRush-Broadlaunch.docx`
- Summary chip: `30 findings.` `31 of 39 library checks ran`
- Severity tally: `High 21` · `Medium 9`
- Tab bar: Overview · **Library Findings (9 open)** · Action (21) · Memo · Redlines · Contract

## Filter row

- All states · All severities · All categories · All topics
- Checkbox: "Hide items needing confirmation"

## Findings list

Heading: **Contract review findings (30)** — `7 of 7 batches complete`

Batches (with check-mark statuses):
- AI Accuracy & Risk (3) ✓
- AI Audit & Incident (2) ✓
- AI Output & IP (3) ✓
- AI Provider Chain (6) ✓
- AI Regulatory Exposure (9) ✓
- AI Training Data Rights (3) ✓
- AI Use Disclosure & Scope (5) ✓

Inline note: "High-severity findings are added to Action Items automatically — you can remove any of them from the action list. Use 'Add to action items' on any other finding to elevate it manually."

---

## AI Provider Chain (5 visible; category total 6)

### HIGH — Auto-added to action items
**AI inference endpoint region undisclosed or outside data-residency scope**
The contract is silent on data-residency for storage and processing and does not address AI inference routing. If Broadlaunch Mobile uses third-party AI models, nothing prevents inference…

### HIGH — Auto-added to action items
**AI regulatory penalties not carved out of liability cap**
Liability is capped at the greater of amounts paid/owing or $100,000, with carve-outs limited to confidentiality breaches, IP infringement, and indemnity. There is no carve-out or super-cap for…

### HIGH — Auto-added to action items
**Upstream foundation-model provider undisclosed**
The contract is silent on whether Broadlaunch Mobile uses any third-party foundation models and does not disclose upstream AI providers, their terms, or data-handling locations. This obscures th…

### MEDIUM
**AI exclusions in cyber / E&O / media-liability insurance**
The contract is silent on insurance requirements and contains no assurance that Broadlaunch Mobile's cyber, tech E&O, or media-liability policies cover AI-related risks or AI-generated content…

### MEDIUM
**Flow-down terms missing for model vendors**
The contract is silent on flowing down privacy, confidentiality, no-training, residency, and breach-notification obligations to any upstream AI/model vendors Broadlaunch Mobile may use. This…

## AI Audit & Incident (2)

### MEDIUM
**No AI incident reporting obligation**
The contract is silent on AI-specific incident reporting by Broadlaunch Mobile (e.g., material model failures, bias incidents, prohibited-output events like CSAM or unlawful deepfakes,…

### MEDIUM
**No audit, red-team, or bias-testing rights**
The contract is silent on any PixelRush Studios rights to audit, red-team, or review bias/safety testing for AI systems Broadlaunch Mobile may use in the Mobile Application. Without these rights…

## AI Accuracy & Risk (3)

### HIGH — Auto-added to action items
**No human-in-the-loop requirement for high-stakes decisions**
The contract is silent on any requirement that AI-assisted decisions in high-stakes contexts (e.g., employment, credit, healthcare, housing, benefits, insurance) receive qualified human review…

### MEDIUM
**No accuracy disclaimer or hallucination risk acknowledgment**
The contract is silent on any AI output accuracy or hallucination disclaimer. If Broadlaunch Mobile incorporates AI-generated content or recommendations in the Mobile Application or related…

### MEDIUM
**No model-change notification**
The contract is silent on model-change notification or change-management if Broadlaunch Mobile adds or modifies AI models, prompts, or training/grounding data in the Mobile Application after…

## AI Training Data Rights (3)

### HIGH — Auto-added to action items
**No-training obligation does not survive termination**
The contract is silent on any no-training obligation and consequently on its survival. Section 5.7 lists surviving provisions (confidentiality, warranties, payments) but does not address…

### HIGH — Auto-added to action items
**Training on customer data without explicit consent**
The contract is silent on prohibiting Broadlaunch Mobile from using end-user data, content, prompts/outputs, or gameplay telemetry to train, fine-tune, evaluate, or benchmark AI models, an…

### MEDIUM
**Derived / usage data used for training**
The contract is silent on whether Broadlaunch Mobile may use telemetry, analytics, "derived," "service," "usage," or aggregated/anonymized data from the GemCrush Saga mobile app to train o…

## AI Use Disclosure & Scope (5)

### HIGH — Auto-added to action items
**AI agent authority and scope not defined**
The contract is silent on agentic AI. If Broadlaunch Mobile deploys AI agents in the Mobile Application (e.g., initiating purchases, messaging users, modifying records, interacting with…

### HIGH — Auto-added to action items
**AI feature disclosure missing or vague**
The contract is silent on whether any features of the Mobile Application use AI/ML, the type of AI, whether such features are on by default, and which third-party models/providers are involved.…

### HIGH — Auto-added to action items
**User-facing AI not labeled**
The contract is silent on labeling or watermarking AI-generated content presented to end users. Since California law governs and the Territory includes EMEA, compliance with U.S./California and…

### MEDIUM
**AI use prohibition absent when no AI declared**
The contract is silent on prohibiting Broadlaunch Mobile from using PixelRush Studios, Inc.'s materials or any related data to train or fine-tune AI/ML systems. There is no carve-out preventing…

### MEDIUM
**Scope of AI processing undefined**
The contract is silent on the scope of any AI processing (inputs, outputs, access, retention, reuse, and downstream use). If Broadlaunch Mobile adds AI features, there are no limits on what data the…

## AI Regulatory Exposure (9)

### HIGH — Auto-added to action items
**California AB 2013 training-data transparency obligations not addressed**
The contract is silent on California AB 2013 (Cal. Bus. & Prof. Code § 22757 et seq.) training-data transparency obligations and who bears compliance costs/risks if Broadlaunch Mobile develops or…

### HIGH — Auto-added to action items
**California ADMT regulations under CCPA not addressed**
The contract is silent on California CCPA/CPPA Automated Decisionmaking Technology (ADMT) regulations (pre-use notice, access, opt-out, risk assessments, cybersecurity audits) and does no…

### HIGH — Auto-added to action items
**California AI Transparency Act (SB 942 / CAITA) watermarking and detection obligations not addressed**
The contract is silent on California AI Transparency Act (SB 942/CAITA) obligations (manifest/latent disclosures, C2PA provenance/watermarking, AI-detection tool availability, and required…

### HIGH — Auto-added to action items
**Colorado AI Act consumer-AI duties not addressed**
The contract is silent on Colorado AI Act (ADAI) obligations and role allocation (developer vs. deployer), including duty of reasonable care to avoid algorithmic discrimination, consumer notices…

### HIGH — Auto-added to action items
**EU AI Act GPAI-provider obligations not flowed through**
The contract is silent on EU AI Act GPAI obligations and flow-downs. If the Mobile Application uses or provides a General-Purpose AI model in the EU, there is no requirement for Broadlaunch Mobile…

### HIGH — Auto-added to action items
**EU AI Act prohibited-use and high-risk obligations not addressed**
The contract is silent on EU AI Act prohibited-use bans and high-risk obligations, Art. 50 transparency (chatbots/synthetic media), and allocation of provider/deployer/distributor roles for…

### HIGH — Auto-added to action items
**GDPR Article 22 automated-decision-making safeguards missing**
The contract is silent on GDPR Article 22 safeguards (meaningful information about logic, human intervention, ability to express a view and contest) if any automated decisions with legal or similarl…

### HIGH — Auto-added to action items
**NYC Local Law 144 (AEDT) obligations not allocated**
The contract is silent on NYC Local Law 144 (AEDT) obligations (independent bias audit, candidate notice, public disclosure, opt-out/alternative process, record retention) and does not allocate…

### HIGH — Auto-added to action items
**Texas Responsible AI Governance Act (TRAIGA) prohibited-use risk not addressed**
The contract is silent on Texas Responsible AI Governance Act (TRAIGA) prohibited-use representations (e.g., intent to incite harm, unlawful discrimination, CSAM/deepfakes) and…

## AI Output & IP (3)

### HIGH — Auto-added to action items
**AI output indemnity gap**
The contract is silent on whether Broadlaunch Mobile's IP indemnity expressly covers infringement or other claims arising from outputs of any generative AI features embedded in or used to provide…

### HIGH — Auto-added to action items
**AI output ownership ambiguous**
The contract is silent on ownership and rights in any AI-generated outputs (e.g., in-app text, images, music, levels, or other content generated by or with AI). While Section 14 allocates…

### HIGH — Auto-added to action items
**Training-data lawfulness warranty missing**
The contract is silent on a warranty from Broadlaunch Mobile that any AI/ML models used (including third-party APIs or services) were trained on data lawfully obtained and used in compliance with…
