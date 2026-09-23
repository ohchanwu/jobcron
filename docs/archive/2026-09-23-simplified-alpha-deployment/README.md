# Superseded First-Production Deployment Records

These records designed and hardened Jobcron's original autonomous, multi-slice
production launch. They are preserved because they contain useful security,
recovery, Terraform, and implementation rationale.

For the initial invite-only alpha, that execution model became disproportionate
to the expected audience and one-day attended launch window. The project moved
to a smaller, non-autonomous process in which the human operator remains present
for console inspection, authentication, sensitive inputs, and every external
mutation approval.

The active launch contract is the
[human-assisted alpha deployment specification](../../specs/260923-human-assisted-alpha-deployment.md).
It retains the important safety boundaries: a fresh reviewed plan, exact
artifact, private verification, tested recovery, explicit public-cutover
approval, and data-safe rollback. It defers generalized automation and
high-availability work rather than weakening those boundaries.

The files in this directory are historical and must not be executed as the
current runbook. In particular, their saved-plan assumptions predate stopped
compute/database resources and deletion of the reserved EIP. Generate fresh
plans from reconciled live state.

Archived records:

- PostgreSQL first-production deployment implementation plan
- Terraform AWS foundation and Cloudflare ingress automation specification
- Terraform-first launch human-blocked steps
- Terraform-first launch roadmap
- two-window autonomous authorization decision
- Pre-Batch-1 human input checklist
- Pre-Batch-1 Window 1 authorization contract
- Terraform Slice 4 replacement-host implementation plan
- Terraform Slice 5 edge-automation implementation plan
- production custody P1 repairs plan
