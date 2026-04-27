# Overview
Accorldli is a b2b app for lawyers in various segments:
* independent practioners
* in-house legal counsel
* law firms

## User Management Structure
Because of our wide variety of customer segments, we are going to need a fairly complex user structure that can handle large organizations, but also individuals.
Organization: a company or org (required)
    Department: a group within the org. One org may have many departments. (optional)
        User: an individual human, who may be part of 1 department, and is always part of 1 organization.
    

## Pricing
Our product burns tokens in large quantities, as part of it's essential functionality. To make this feasible, we must tie our pricing to usage. However, that the granularity of usage must be intuitive to our customers, so they can predict and control their spend.

To achieve this, we are planning to charge "per contract" in tiers. Here's an example system:

### Independent Practioners
Tier 1: Pro
    $200/month
    10 contract analyses
    unlimited reports and memos

Tier 2: Gold
    $400/month
    25 contract analyses
    unlimited reports and memos

Extra Contract Packs
    $100 per 10 contracts
    Expire in 1 year

## Billing
Must register a credit card (or PayPal) and are billing every month on the day of their signing.
If they cancel, their account is active until the last day of the period.
If they request a refund, they may get an automatic refund in the first 7 days of the period, if the have analyzed 2 or fewer contracts.

### Teams
Tier 2: Small Team of 3 lawyers
    $600/month
    3 seats
    40 contract analyses across group
    unlimited reports and memos
    Team Dashboard

Tier 3: Large Team of 10 lawyers
    $2000/month
    10 seats
    130 contract analyses across group
    unlimited reports and memos
    Team Dashboard

Enterprise Tier
    They must call. It will be a custom combination of seats, contract analyses, and Team Dashboard

