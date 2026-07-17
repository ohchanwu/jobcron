# Campaign Scope Must Outlive Slice Completion

## Incident

The human authorized execution of the complete Ponytail campaign. The campaign contained ten
ordered implementation batches, `PT4-001` through `PT4-010`.

After completing and independently verifying `PT4-006`, the agent stopped and reported the
current slice as complete. It did not return to the campaign ledger, integrate the reviewed
slice, or continue with `PT4-007` through `PT4-010`.

## Why Stopping Was Wrong

`PT4-006` was a child task inside the authorized campaign, not the campaign's terminal task.
Completing a hooked bead satisfies that bead's acceptance criteria; it does not replace the
parent objective.

The slice's `no push`, `no merge`, `no cleanup`, and `no gt done` boundaries governed how that
delivery package could be handled. They did not revoke authorization to continue coordinating
the remaining campaign work.

There was no technical blocker, destructive operation, or unresolved product decision that
required human input.

## Root Cause

The agent substituted the active slice's terminal state for the top-level campaign state. It
generated its completion report from the slice evidence without reconciling that result against
the authoritative reduction ledger.

This is a scope-tracking failure: a child task became the working horizon, and the parent goal
was lost when the child finished.

## Required Behavior

For any multi-slice campaign with standing authorization:

1. Treat the campaign plan or ledger as the persistent parent objective.
2. After each child task, record its result and immediately re-read the parent ledger.
3. Distinguish clearly among `slice complete`, `integrated`, and `campaign complete`.
4. Continue with the next ready slice unless a real blocker or authorized stop condition exists.
5. Obey a child's delivery boundaries without treating them as permission to stop the campaign.
6. Before issuing a final completion report, verify that every planned slice is implemented,
   reviewed, integrated, documented, and archived as required.
7. If work remains, report the exact remaining slice count instead of using unqualified language
   such as "complete" or "ready for integration."

The safe default is: child completion returns control to the parent plan; it never silently ends
the parent plan.
