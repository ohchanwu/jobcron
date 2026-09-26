"""Read-only D1 lifecycle schema/consumer; no writer import or activation path.

The selected register is trusted attended custody, not an anti-rollback device.
Approval here means independent diagnostic review only, never apply approval.
"""
import datetime
import hashlib
import re
from typing import Any

from private_plan_shape import ShapeError, require

REGISTER_SCHEMA = "jobcron-private-plan-shape-review-register-v1"
COMMITS = frozenset(("checker_commit", "extractor_commit", "shape_contract_commit", "approved_release_commit"))
TOOLS = dict(terraform_version="1.15.8", terraform_json_format="1.2",
             aws_provider_version="6.33.0", aws_schema_contract="aws-6.33.0-plan-1.2-v1")
REASONS = {"approved": frozenset(("independent_review",)),
           "retired": frozenset(("superseded", "abandoned")),
           "invalidated": frozenset(("freshness_failed", "custody_lost", "parser_defect",
                                     "regeneration_failed", "incomplete_coverage", "source_changed"))}
EVENT_KEYS = frozenset(("sequence", "status", "binding", "manifest_sha256", "reviewer", "writer", "timestamp", "reason"))


def digest(value, length):
    require(type(value) is str and re.fullmatch("[0-9a-f]{" + str(length) + "}", value), "invalid_binding")


def binding_valid(binding):
    require(type(binding) is dict and set(binding) == COMMITS | set(TOOLS) | {"binary_plan_sha256"}, "invalid_binding")
    for key in COMMITS:
        digest(binding[key], 40)
    digest(binding["binary_plan_sha256"], 64)
    require(all(binding[key] == value for key, value in TOOLS.items()), "unsupported_format")


def validate(register: Any):
    require(type(register) is dict and set(register) == {"schema", "events"} and
            register["schema"] == REGISTER_SCHEMA and type(register["events"]) is list, "invalid_register")
    previous = 0
    for event in register["events"]:
        require(type(event) is dict and set(event) == EVENT_KEYS, "invalid_register")
        require(type(event["sequence"]) is int and event["sequence"] > previous, "invalid_sequence")
        previous = event["sequence"]
        binding_valid(event["binding"])
        digest(event["manifest_sha256"], 64)
        state = event["status"]
        require(type(state) is str and state in REASONS and type(event["reason"]) is str and
                event["reason"] in REASONS[state], "invalid_status")
        require(event["reviewer"] == "reviewer-sol" and event["writer"] == "default-orchestrator", "invalid_role")
        stamp = event["timestamp"]
        require(type(stamp) is str and re.fullmatch(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,6})?(?:Z|[+-]\d{2}:\d{2})", stamp), "invalid_timestamp")
        try:
            datetime.datetime.fromisoformat(stamp.replace("Z", "+00:00"))
        except ValueError:
            raise ShapeError("invalid_timestamp") from None



def status(register, binding, manifest_bytes):
    binding_valid(binding)
    validate(register)
    manifest_digest = hashlib.sha256(manifest_bytes).hexdigest()
    state = "unreviewed"
    for event in register["events"]:
        if event["binding"] == binding and event["manifest_sha256"] == manifest_digest:
            state = event["status"]
    # This constant is returned even for historically valid snapshots: a
    # stateless reader cannot prove that no later event exists elsewhere.
    return state, "rollback_detection_unavailable"
