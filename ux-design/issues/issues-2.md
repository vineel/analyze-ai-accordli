# issues-2.pdf — Action Items view

Source PDF: `issues-2.pdf` (4 pages)
Screen: **Action** tab on the same contract review.

## Page chrome

- Document header: `01_MobileDistribution_PixelRush-Broadlaunch.docx`
- Summary: `30 findings.` `31 of 39 library checks ran` · High 21 · Medium 9
- Tab bar: Overview · Library Findings (9 open) · **Action (21)** · Memo · Redlines · Contract
- Sub-tabs: **Action Items 21** · AI Diligence Questions 13 · Internal Decision Points 13

Top blurb: "Items here were elevated automatically (high-severity library findings, and policy items not fully met) or added manually. Review, resolve, or remove anything that doesn't apply."

Each row carries the badges: severity (HIGH), `Conditional` (when present), and `Library`. Each shows the contract location ("Throughout" or e.g. "12.1 Limitation of Liability") and a `Basis:` line — either "Contract is silent on this topic" or "Depends on the answer to: …" (linking to a diligence question), or a verbatim contract section.

## Action items (21)

### HIGH — `Library`
**AI inference endpoint region undisclosed or outside data-residency scope**
Throughout · Basis: Contract is silent on this topic
The contract is silent on data-residency for storage and processing and does not address AI inference routing. If Broadlaunch Mobile uses third-party AI models, nothing prevents inference…

### HIGH — `Library`
**User-facing AI not labeled**
Throughout · Basis: Contract is silent on this topic
The contract is silent on labeling or watermarking AI-generated content presented to end users. Since California law governs and the Territory includes EMEA, compliance with U.S./California and…

### HIGH — `Conditional` `Library`
**NYC Local Law 144 (AEDT) obligations not allocated**
Throughout · Basis: Depends on the answer to: "Will Broadlaunch Mobile avoid using an AEDT for hiring or pro…"
The contract is silent on NYC Local Law 144 (AEDT) obligations (independent bias audit, candidate notice, public disclosure, opt-out/alternative process, record retention) and does not allocate…

### HIGH — `Conditional` `Library`
**Texas Responsible AI Governance Act (TRAIGA) prohibited-…**
Throughout · Basis: Depends on the answer to: "Will the Mobile Application avoid any AI functionality develope…"
The contract is silent on Texas Responsible AI Governance Act (TRAIGA) prohibited-use representations (e.g., intent to incite harm, unlawful discrimination, CSAM/deepfakes) and…

### HIGH — `Conditional` `Library`
**No human-in-the-loop requirement for high-stakes decisions**
Throughout · Basis: Depends on the answer to: "Will Broadlaunch Mobile use any AI in employment, credit, hea…"
The contract is silent on any requirement that AI-assisted decisions in high-stakes contexts (e.g., employment, credit, healthcare, housing, benefits, insurance) receive qualified human review…

### HIGH — `Library`
**No-training obligation does not survive termination**
Throughout · Basis: Contract is silent on this topic
The contract is silent on any no-training obligation and consequently on its survival. Section 5.7 lists surviving provisions (confidentiality, warranties, payments) but does not address…

### HIGH — `Library`
**Training on customer data without explicit consent**
Throughout · Basis: Contract is silent on this topic
The contract is silent on prohibiting Broadlaunch Mobile from using end-user data, content, prompts/outputs, or gameplay telemetry to train, fine-tune, evaluate, or benchmark AI models, an…

### HIGH — `Library`
**AI agent authority and scope not defined**
Throughout · Basis: Contract is silent on this topic
The contract is silent on agentic AI. If Broadlaunch Mobile deploys AI agents in the Mobile Application (e.g., initiating purchases, messaging users, modifying records, interacting with…

### HIGH — `Library`
**AI feature disclosure missing or vague**
Throughout · Basis: Contract is silent on this topic
The contract is silent on whether any features of the Mobile Application use AI/ML, the type of AI, whether such features are on by default, and which third-party models/providers are involved.…

### HIGH — `Library`
**AI regulatory penalties not carved out of liability cap**
12.1 Limitation of Liability · Basis: Contract §12.1 Limitation of Liability (verbatim)
Liability is capped at the greater of amounts paid/owing or $100,000, with carve-outs limited to confidentiality breaches, IP infringement, and indemnity. There is no carve-out or super-cap for…

### HIGH — `Library`
**Upstream foundation-model provider undisclosed**
Throughout · Basis: Contract is silent on this topic
The contract is silent on whether Broadlaunch Mobile uses any third-party foundation models and does not disclose upstream AI providers, their terms, or data-handling locations. This obscures t…

### HIGH — `Conditional` `Library`
**AI output indemnity gap**
Throughout · Basis: Depends on the answer to: "Will Broadlaunch Mobile refrain from including any generative …"
The contract is silent on whether Broadlaunch Mobile's IP indemnity expressly covers infringement or other claims arising from outputs of any generative AI features embedded in or used to provid…

### HIGH — `Conditional` `Library`
**AI output ownership ambiguous**
Throughout · Basis: Depends on the answer to: "Will the Mobile Application avoid generating any end-user-visi…"
The contract is silent on ownership and rights in any AI-generated outputs (e.g., in-app text, images, music, levels, or other content generated by or with AI). While Section 14 allocates…

### HIGH — `Conditional` `Library`
**Training-data lawfulness warranty missing**
Throughout · Basis: Depends on the answer to: "Will Broadlaunch Mobile refrain from embedding or relying on …"
The contract is silent on a warranty from Broadlaunch Mobile that any AI/ML models used (including third-party APIs or services) were trained on data lawfully obtained and used in…

### HIGH — `Conditional` `Library`
**California AB 2013 training-data transparency obligations n…**
Throughout · Basis: Depends on the answer to: "Will the Mobile Application avoid any generative-AI system ma…"
The contract is silent on California AB 2013 (Cal. Bus. & Prof. Code § 22757 et seq.) training-data transparency obligations and who bears compliance costs/risks if Broadlaunch Mobile develops o…

### HIGH — `Conditional` `Library`
**California ADMT regulations under CCPA not addressed**
Throughout · Basis: Depends on the answer to: "Will Broadlaunch Mobile deploy ADMT affecting Californians i…"
The contract is silent on California CCPA/CPPA Automated Decisionmaking Technology (ADMT) regulations (pre-use notice, access, opt-out, risk assessments, cybersecurity audits) and does n…

### HIGH — `Conditional` `Library`
**California AI Transparency Act (SB 942 / CAITA) watermarki…**
Throughout · Basis: Depends on the answer to: "Will the Mobile Application avoid generating or distributing sy…"
The contract is silent on California AI Transparency Act (SB 942/CAITA) obligations (manifest/latent disclosures, C2PA provenance/watermarking, AI-detection tool availability, and required…

### HIGH — `Conditional` `Library`
**Colorado AI Act consumer-AI duties not addressed**
Throughout · Basis: Depends on the answer to: "Will the Mobile Application avoid using AI for "consequential d…"
The contract is silent on Colorado AI Act (ADAI) obligations and role allocation (developer vs. deployer), including duty of reasonable care to avoid algorithmic discrimination, consumer notice…

### HIGH — `Conditional` `Library`
**EU AI Act GPAI-provider obligations not flowed through**
Throughout · Basis: Depends on the answer to: "Will the Mobile Application avoid using or providing any GPAI …"
The contract is silent on EU AI Act GPAI obligations and flow-downs. If the Mobile Application uses or provides a General-Purpose AI model in the EU, there is no requirement for Broadlaunch Mobil…

### HIGH — `Conditional` `Library`
**EU AI Act prohibited-use and high-risk obligations not addr…**
Throughout · Basis: Depends on the answer to: "Will the Mobile Application avoid making any AI features availa…"
The contract is silent on EU AI Act prohibited-use bans and high-risk obligations, Art. 50 transparency (chatbots/synthetic media), and allocation of provider/deployer/distributor roles for…

### HIGH — `Conditional` `Library`
**GDPR Article 22 automated-decision-making safeguards mi…**
Throughout · Basis: Depends on the answer to: "Will the Mobile Application avoid automated decisions that pro…"
The contract is silent on GDPR Article 22 safeguards (meaningful information about logic, human intervention, ability to express a view and contest) if any automated decisions with legal or…
