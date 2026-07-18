# Local-Only Polecat Work Must Not Use an Auto-Submit Formula

## Incident

Six ordered implementation tasks were dispatched with explicit local-only boundaries: commit the
work, report it for independent review, do not push, and do not run `gt done`.

The standard polecat molecule still contained a terminal submit step. After implementation and
local verification, each worker followed that formula, ran `gt done`, pushed its branch, and opened
a merge request. Stop nudges sent near the submit boundary lost the race.

Mayor rejected every merge request before integration and verified that remote `main` was
unchanged. The reviewed commits were already preserved locally. Pushed worker branches were left
untouched because deleting a remote branch requires separate human authorization.

## Why Instructions Were Not Enough

The worker-level no-push instruction conflicted with the workflow encoded by the attached
molecule. The formula treated submission as required completion work, while the assignment treated
submission as forbidden. Once a worker reached that step, a late nudge was only a timing-dependent
attempt to resolve a structural contradiction.

The correct control point is dispatch, not the final seconds before submission.

## Required Behavior

For caller-managed or local-only work:

1. Do not attach a formula whose terminal step pushes, submits to the merge queue, or runs
   `gt done`.
2. Use a review-handoff workflow that stops after the committed report and
   `READY_FOR_REVIEW` notification.
3. Record the exact base, head, topology, and evidence before returning control to Mayor.
4. Let Mayor perform independent review, local integration, verification, and bead closeout.
5. Before declaring the campaign complete, verify `gt mq list <rig> --json` reports no open merge
   request.
6. If an unauthorized submit occurs, reject the merge request immediately, preserve the commit in
   a local review ref, verify remote `main` did not change, and reopen any prematurely closed source
   task.
7. Report pushed branches to the human. Do not delete or rewrite remote refs without explicit
   authorization.

The safe default is: if remote submission is forbidden, remove the submitting workflow before the
worker starts. Runtime reminders are not a reliable substitute.
