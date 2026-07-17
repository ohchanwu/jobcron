# Brainstorming Skill and Autonomous-Mode Question Gates

## Incident

During the Ponytail campaign, the Greenhouse live gate exposed a narrow prerequisite bug:
Daangn's legacy posting URLs redirect to a new canonical host and path. The repair was small,
reversible, and inside the already authorized implementation campaign.

The agent invoked `superpowers:brainstorming` because the repair changes observable URL behavior.
That skill's hard gate required a design approval before implementation, so the agent stopped the
campaign and asked the human to approve the obvious canonical-URL repair.

## Why the Interruption Was Wrong

The repository's autonomous-mode question budget permits interruption only for destructive
irreversible work, an unresolved product trade-off, or an explicit classifier stop. This repair
was none of those:

- the correct destination was verified in a real browser;
- the old URL was demonstrably stale;
- the change was independently reversible;
- the user had already authorized executing the campaign and repairing pending plans; and
- the recommended choice preserved, rather than expanded, product behavior.

The interruption added review latency without protecting a meaningful human decision.

## Root Cause

Two instructions interacted badly:

1. `superpowers:brainstorming` treats every behavior modification as creative work and requires
   explicit approval.
2. Autonomous mode expects agents to decide routine implementation details and report them after
   the fact.

This was an autonomous-mode classification bug. Routine, reversible corrective work inside
existing authorization must not trigger brainstorming's stop/question gate; changes to product
scope or unresolved product trade-offs still must. The agent followed the first rule mechanically
without applying the second rule's distinction between a product decision and a routine corrective
implementation choice.

## Required Behavior

For an already authorized implementation campaign:

- Do not interrupt for a narrow prerequisite bugfix when the desired behavior is already proven,
  the repair is reversible, and no product trade-off remains.
- Make the recommended implementation choice, keep it in a separate commit, update dependent
  plans, and list the decision in the final report.
- Use brainstorming when the discovery changes product scope, introduces competing user-visible
  behaviors, or leaves success criteria genuinely ambiguous.

The skill integration should eventually encode this distinction. A safe fix would let autonomous
mode auto-approve the recommended path for routine, reversible corrective work inside an existing
authorization while preserving the hard gate for new features and real product choices.
