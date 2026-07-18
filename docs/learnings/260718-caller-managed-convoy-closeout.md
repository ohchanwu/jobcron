# Caller-Managed Convoys Require Mayor Closeout

## Incident

Two Daangn convoys completed a canonical role URL repair and its independent review. The
implementation polecat reported a clean verified commit, and the reviewer returned `APPROVE`
with no findings. The Mayor integrated the exact source commit and archived the implementation
records. The worker tasks nevertheless remained hooked, their sessions stopped, and the convoys
continued to show `0/1` completed.

During later campaign closeout, the Mayor classified the convoys as unrelated and left their GT
state untouched. It then failed to inspect the commit graph and incorrectly told the human that
the approved commit still required integration. The actual remaining work was terminal convoy
bookkeeping.

## Why Stopping Was Wrong

The convoys were caller-managed with local integration. The polecat instructions explicitly
prohibited the workers from merging, closing beads, cleaning up, or running `gt done`. Those
restrictions transferred integration and bookkeeping responsibility to the Mayor; they did not
make the completed work somebody else's responsibility.

The stopped sessions were therefore expected after delivery. The hooked tasks and `0/1` convoy
counters described unfinished coordination, not unfinished implementation. Completion mail and
the verified commit showed that the work was ready for Mayor action.

## Root Cause

The Mayor confused “outside the current campaign” with “outside Mayor responsibility.” It
trusted the convoy counter without reconciling the assigned polecats, issue notes, review mail,
and commit ancestry. This hid completed integration behind stale orchestration state and caused
an incorrect status report.

## Required Behavior

Before declaring a rig or campaign complete, the Mayor must:

1. Inspect every owned or caller-managed convoy, including work outside the current campaign.
2. Check each assigned polecat's live status, issue notes, local branch, and completion mail.
3. Treat `no merge`, `no cleanup`, and `no gt done` as worker boundaries unless they explicitly
   constrain the Mayor too.
4. Verify commit ancestry before claiming that reviewed work is or is not integrated.
5. When implementation and review are complete but integration is absent, validate the exact
   commit, integrate it locally, and rerun the required gates.
6. Close the implementation task, review task, attached molecules, and convoys after successful
   integration.
7. Distinguish a real blocker from stale bookkeeping. A stopped worker with a verified delivery
   report is waiting for closeout, not necessarily stalled on implementation.
8. Leave the rig running only for genuinely active work, not because completed caller-managed
   convoys were never reconciled.

The safe default is: caller-managed delivery returns control to the Mayor, who owns integration
and terminal bookkeeping unless the assignment explicitly says otherwise.
