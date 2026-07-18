# Caller-Managed Convoys Require Mayor Closeout

## Incident

Two Daangn convoys completed a canonical role URL repair and its independent review. The
implementation polecat reported a clean verified commit, and the reviewer returned `APPROVE`
with no findings. Their sessions then stopped while the tasks remained hooked and the convoys
continued to show `0/1` completed.

The Mayor classified the convoys as unrelated to the Ponytail campaign and left them untouched.
It later reported that all remaining work was complete without integrating the approved commit or
closing the convoy bookkeeping.

## Why Stopping Was Wrong

The convoys were caller-managed with local integration. The polecat instructions explicitly
prohibited the workers from merging, closing beads, cleaning up, or running `gt done`. Those
restrictions transferred the final integration and bookkeeping responsibility to the Mayor; they
did not make the completed work somebody else's responsibility.

The stopped sessions were therefore expected after delivery. The hooked tasks and `0/1` convoy
counters described unfinished coordination, not unfinished implementation. Completion mail and
the verified commit showed that the work was ready for Mayor action.

## Root Cause

The Mayor confused “outside the current campaign” with “outside Mayor responsibility.” It
trusted the convoy counter without checking the assigned polecats, issue notes, local commits,
and review mail. This allowed stale orchestration state to hide a completed delivery package.

## Required Behavior

Before declaring a rig or campaign complete, the Mayor must:

1. Inspect every owned or caller-managed convoy, including work outside the current campaign.
2. Check each assigned polecat's live status, issue notes, local branch, and completion mail.
3. Treat `no merge`, `no cleanup`, and `no gt done` as worker boundaries unless they explicitly
   constrain the Mayor too.
4. When implementation and independent review are complete, validate the exact commit, integrate
   it locally, and rerun the required gates.
5. Close the implementation task, review task, attached molecules, and convoys after successful
   integration.
6. Distinguish a real blocker from stale bookkeeping. A stopped worker with a verified delivery
   report is waiting for closeout, not necessarily stalled on implementation.
7. Leave the rig running only for genuinely active work, not because completed caller-managed
   convoys were never reconciled.

The safe default is: caller-managed delivery returns control to the Mayor, who owns integration
and terminal bookkeeping unless the assignment explicitly says otherwise.
