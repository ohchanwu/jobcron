"""D1 value-blind structural inventory. Not a semantic plan validator.

This module accepts an in-memory decoded document; it never opens evidence,
invokes Terraform, writes a review register, or grants deployment approval.
"""
import json
import math
import time
from typing import Any


MAX_BYTES = 32 * 1024 * 1024
MAX_NODES = 200000
MAX_DEPTH = 80
SCHEMA = "jobcron-private-plan-shape-v1"

# Context-specific public vocabulary, authored independently of any saved plan.
# An unclassified field is still traversed, but its spelling is never exported.
# Locations refer to the unchanged canonical checker, not inferred policy.
CONTEXTS = {
    "plan": ("format_version terraform_version variables planned_values resource_changes "
             "resource_drift prior_state configuration output_changes checks diagnostics "
             "timestamp applyable complete errored relevant_attributes"),
    "state": "format_version terraform_version values",
    "values": "outputs root_module",
    "module": "resources child_modules address",
    "configuration": "provider_config root_module",
    "config_module": "resources outputs variables module_calls",
    "resource": "address mode type name index provider_name schema_version values sensitive_values depends_on",
    "config_resource": "address mode type name provider_config_key expressions schema_version count_expression for_each_expression depends_on",
    "change_resource": "address previous_address module_address mode type name index provider_name deposed change action_reason",
    "change": "actions before after after_unknown before_sensitive after_sensitive replace_paths importing generated_config before_identity after_identity",
    "expression": "constant_value references",
    "output": "sensitive type value",
    "variable": "value",
    "diagnostic": "severity summary detail range snippet",
    "provider": "name full_name alias version_constraint expressions module_address",
    "check": "address status instances",
}
CONTEXTS = {key: frozenset(value.split()) for key, value in CONTEXTS.items()}
VARIABLES = frozenset(("canonical_network_config", "private_database_config",
                       "replacement_host_ami_id", "replacement_public_subnet_key"))
OUTPUTS = frozenset(("replacement_instance_id",))
# Only checker-explicit value paths receive public names. Other provider fields
# remain opaque/unclassified even if Terraform happens to understand them.
ATTRS = {
    "aws_instance": {
        "": "id ami instance_type subnet_id iam_instance_profile key_name associate_public_ip_address vpc_security_group_ids metadata_options root_block_device user_data public_ip public_dns tags",
        "metadata_options": "http_endpoint http_tokens http_put_response_hop_limit instance_metadata_tags",
        "root_block_device": "encrypted volume_type volume_size delete_on_termination device_name iops kms_key_id tags tags_all throughput volume_id",
    },
    "aws_eip": {"": "id allocation_id domain instance network_interface associate_with_private_ip association_id"},
    "aws_db_instance": {"": "id identifier publicly_accessible storage_encrypted deletion_protection manage_master_user_password backup_retention_period vpc_security_group_ids latest_restorable_time"},
    "aws_security_group": {"": "id ingress", "ingress": "from_port to_port protocol security_groups cidr_blocks ipv6_cidr_blocks prefix_list_ids self"},
    "aws_vpc_security_group_ingress_rule": {"": "security_group_id referenced_security_group_id from_port to_port ip_protocol cidr_ipv4 cidr_ipv6 prefix_list_id"},
    "aws_iam_role": {"": "id name tags inline_policy managed_policy_arns"},
    "aws_iam_instance_profile": {"": "id name role tags"},
    "aws_iam_role_policy": {"": "role policy"},
    "aws_iam_role_policy_attachment": {"": "role policy_arn"},
    "aws_subnet": {"": "id"},
}
ATTRS = {resource: {path: frozenset(keys.split()) for path, keys in paths.items()}
         for resource, paths in ATTRS.items()}
ADDRESSES = frozenset((
    "aws_iam_role.replacement_host", "aws_iam_role_policy_attachment.replacement_host_ssm",
    "aws_iam_role_policy.replacement_host_runtime", "aws_iam_instance_profile.replacement_host",
    "aws_instance.replacement_host", "aws_vpc.canonical", "aws_internet_gateway.canonical",
    "aws_route_table.public", "aws_route.public_ipv4_default", "aws_eip.origin",
    "aws_route_table.database", "aws_security_group.origin", "aws_security_group.database",
    "aws_vpc_security_group_ingress_rule.database_postgresql_from_origin",
    "aws_db_subnet_group.production", "aws_db_parameter_group.production", "aws_db_instance.production",
    "aws_secretsmanager_secret.runtime", "aws_s3_bucket.recovery",
    "aws_s3_bucket_public_access_block.recovery", "aws_s3_bucket_versioning.recovery",
    "aws_s3_bucket_server_side_encryption_configuration.recovery", "aws_s3_bucket_policy.recovery",
    "aws_s3_bucket_lifecycle_configuration.recovery",
    *(f'aws_subnet.public["public_{key}"]' for key in "abcd"),
    *(f'aws_subnet.database["database_{key}"]' for key in "ab"),
    *(f'aws_route_table_association.database["database_{key}"]' for key in "ab"),
))
TYPES = frozenset(address.split(".")[0] for address in ADDRESSES)
# Pinned public AWS 6.33.0 `providers schema -json`, independently decoded
# offline. This is a code contract, never learned from candidate-plan values.
PROVIDER_SCHEMAS = dict.fromkeys(TYPES, 0)
PROVIDER_SCHEMAS.update(aws_db_instance=2, aws_instance=2, aws_security_group=1,
                        aws_subnet=1, aws_vpc=1, aws_s3_bucket_lifecycle_configuration=1)
ACTIONS = frozenset(("no-op", "create", "read", "update", "delete", "forget"))
REQUIRED = {"format_version": "string", "terraform_version": "string",
            "resource_changes": "array", "output_changes": "object",
            "variables": "object", "configuration": "object"}

# Explicit structural obligations from canonical body lines 336-438. Actual
# scalar values and cross-resource equality remain canonical semantic checks.
REQUIRED_ATTRS = {
    ("aws_instance", ""): dict(ami="string", instance_type="string", subnet_id="string",
        iam_instance_profile="string", associate_public_ip_address="boolean", vpc_security_group_ids="array",
        metadata_options="array", root_block_device="array", user_data="string"),
    ("aws_instance", "metadata_options"): dict(http_endpoint="string", http_tokens="string", http_put_response_hop_limit="number"),
    ("aws_instance", "root_block_device"): dict(encrypted="boolean", volume_type="string", volume_size="number", delete_on_termination="boolean"),
    ("aws_db_instance", ""): dict(id="string", identifier="string", publicly_accessible="boolean",
        storage_encrypted="boolean", deletion_protection="boolean", manage_master_user_password="boolean",
        backup_retention_period="number", vpc_security_group_ids="array"),
    ("aws_security_group", ""): dict(id="string", ingress="array"),
    ("aws_vpc_security_group_ingress_rule", ""): dict(security_group_id="string", referenced_security_group_id="string",
        from_port="number", to_port="number", ip_protocol="string"),
    ("aws_eip", ""): dict(domain="string", associate_with_private_ip="null"),
}
OBJECT_CONTEXTS = frozenset(CONTEXTS) | {"variables", "outputs", "output_changes", "providers", "expressions"}
ARRAY_CONTEXTS = frozenset(("changes", "resources", "config_resources", "modules", "diagnostics", "checks", "actions", "references"))
CONTEXT_REQUIRED = {
    "change": ("actions", "before", "after"),
    "change_resource": ("address", "mode", "type", "change"),
    "resource": ("address", "mode", "type", "schema_version", "values"),
    "configuration": ("root_module",), "config_module": ("resources",),
    "output_changes": ("replacement_instance_id",),
}


class ShapeError(ValueError):
    """Fixed value-free errors only."""


def require(condition, code="invalid_document"):
    if not condition:
        raise ShapeError(code)


def encode(value):
    return (json.dumps(value, ensure_ascii=True, separators=(",", ":"),
                       sort_keys=True, allow_nan=False) + "\n").encode("ascii")


def parse(raw) -> dict[str, Any]:
    require(type(raw) is bytes and len(raw) <= MAX_BYTES, "input_budget")

    def pairs(items):
        result = {}
        for key, value in items:
            require(key not in result, "duplicate_key")
            result[key] = value
        return result

    def constant(_):
        raise ShapeError("invalid_document")

    def number(value):
        result = float(value)
        require(math.isfinite(result), "invalid_document")
        return result

    try:
        result = json.loads(raw.decode("utf-8"), object_pairs_hook=pairs,
                            parse_constant=constant, parse_float=number)
    except (ValueError, UnicodeError, RecursionError):
        raise ShapeError("invalid_document") from None
    require(type(result) is dict)
    return result


def kind(value):
    return {dict: "object", list: "array", str: "string", int: "number",
            float: "number", bool: "boolean", type(None): "null"}[type(value)]


def inventory(plan) -> dict[str, Any]:
    require(type(plan) is dict)
    require(plan.get("format_version") == "1.2" and
            plan.get("terraform_version") == "1.15.8", "unsupported_format")
    nodes, diffs, resources, relations = [], [], [], []
    counts = dict(classified=0, opaque=0, unsupported=0)
    seen = set()
    planned_addresses = set()
    deadline = time.monotonic() + 10

    def difference(node, category, rule="structure"):
        diffs.append(dict(node=node, category=category, rule=rule))

    def walk(value, parent, edge, context="opaque", resource_type="", attr="", mask=False, depth=0, phase=""):
        require(depth <= MAX_DEPTH and len(nodes) < MAX_NODES and time.monotonic() <= deadline,
                "inventory_incomplete")
        node_id = len(nodes)
        if context == "opaque" and type(value) in (dict, list):
            context = "unknown"
        classification = ("unsupported" if context == "unknown" else
                          "opaque" if context == "opaque" else "classified")
        node = dict(id=node_id, parent=parent, edge=edge, type=kind(value),
                    classification=classification)
        counts[classification] += 1
        if type(value) in (dict, list):
            node["cardinality"] = len(value)
        elif mask and type(value) is bool:
            node["mask"] = "true" if value else "false"
        elif mask:
            difference(node_id, "mask")
        nodes.append(node)
        if context == "move_import" and value is not None:
            difference(node_id, "unclassified", "move_import")
        if type(value) is dict:
            for field in CONTEXT_REQUIRED.get(context, ()):
                if field not in value:
                    diffs.append(dict(node=node_id, category="missing", rule="required_fields", expected=field))
        if context in OBJECT_CONTEXTS and type(value) is not dict:
            difference(node_id, "type")
        if context in ARRAY_CONTEXTS and type(value) is not list:
            # Scalar entries of these collections are checked below instead.
            if not (parent is not None and nodes[parent]["type"] == "array" and context in ("actions", "references")):
                difference(node_id, "type")
        if context == "attrs" and not mask and phase == "after":
            required = REQUIRED_ATTRS.get((resource_type, attr), {})
            if type(value) is dict:
                for field, expected_type in required.items():
                    if field not in value:
                        diffs.append(dict(node=node_id, category="missing", rule="required_fields", expected=field))
                    elif kind(value[field]) != expected_type:
                        diffs.append(dict(node=node_id, category="type", rule="required_fields", expected=field))
            elif type(value) is list and resource_type == "aws_instance" and attr in ("metadata_options", "root_block_device"):
                if len(value) != 1:
                    difference(node_id, "cardinality")
            elif required:
                difference(node_id, "type", "required_fields")
        if context == "unknown":
            difference(node_id, "unclassified")
        if context in ("resource", "config_resource", "change_resource") and type(value) is dict:
            address = value.get("address")
            address_known = type(address) is str and address in ADDRESSES
            candidate_type = value.get("type")
            resource_type = candidate_type if type(candidate_type) is str and candidate_type in TYPES else ""
            descriptor = dict(node=node_id, address=address if address_known else "unrecognized_address",
                              type=resource_type or "unrecognized_type",
                              mode=value.get("mode") if value.get("mode") in ("managed", "data") else "unrecognized_mode")
            resources.append(descriptor)
            if context == "resource" and resource_type:
                if type(value.get("schema_version")) is not int or value["schema_version"] != PROVIDER_SCHEMAS[resource_type]:
                    difference(node_id, "unclassified", "provider_schema")
            if context in ("resource", "change_resource") and value.get("provider_name") != "registry.terraform.io/hashicorp/aws":
                difference(node_id, "reference", "provider_schema")
            if not address_known or not resource_type:
                difference(node_id, "unclassified", "resource_identity")
            if type(address) is str and address_known:
                if resource_type != address.split(".")[0] or value.get("mode") != "managed":
                    difference(node_id, "type", "resource_identity")
                identity = (context, parent, address)
                if identity in seen:
                    difference(node_id, "cardinality", "resource_identity")
                seen.add(identity)
                if parent is not None and nodes[parent]["edge"] == {"key": "resource_changes"}:
                    planned_addresses.add(address)
                    change = value.get("change")
                    actions = change.get("actions") if type(change) is dict else None
                    expected = [["no-op"]]
                    if address == "aws_instance.replacement_host":
                        expected = [["delete", "create"]]
                        if "action_reason" not in value:
                            diffs.append(dict(node=node_id, category="missing", rule="required_fields", expected="action_reason"))
                    elif address == "aws_eip.origin":
                        expected.append(["create"])
                    if actions not in expected:
                        difference(node_id, "action", "initial_actions")
        if type(value) is dict:
            if context in ("attrs", "expressions"):
                allowed = ATTRS.get(resource_type, {}).get(attr, frozenset())
            elif context == "variables":
                allowed = VARIABLES
            elif context in ("outputs", "output_changes"):
                allowed = OUTPUTS
            elif context == "providers":
                allowed = frozenset(("aws",))
            else:
                allowed = CONTEXTS.get(context, frozenset())
            known = sorted(key for key in value if key in allowed)
            unknown = [key for key in value if key not in allowed]
            ordinals = {key: index for index, key in enumerate(unknown)}
            for key in known + unknown:
                admitted = key in allowed
                child_edge = {"key": key} if admitted else {"opaque_key": ordinals[key]}
                child_context, child_attr = "opaque", attr
                child_phase = phase
                child_mask = mask
                if not admitted:
                    child_context = "opaque" if context == "opaque" else "unknown"
                elif context == "plan":
                    child_context = {"variables": "variables", "resource_changes": "changes",
                                     "resource_drift": "changes", "planned_values": "values",
                                     "prior_state": "state", "configuration": "configuration",
                                     "output_changes": "output_changes", "diagnostics": "diagnostics",
                                     "checks": "checks"}.get(key, "opaque")
                elif context == "state" and key == "values":
                    child_context = "values"
                elif context == "values":
                    child_context = "module" if key == "root_module" else "outputs"
                elif context == "configuration":
                    child_context = "config_module" if key == "root_module" else "providers"
                elif context in ("module", "config_module"):
                    child_context = {"resources": "config_resources" if context == "config_module" else "resources",
                                     "child_modules": "modules", "outputs": "outputs", "variables": "variables"}.get(key, "unknown")
                elif context in ("resource", "config_resource", "change_resource"):
                    child_context = {"values": "attrs", "sensitive_values": "attrs", "expressions": "expressions",
                                     "change": "change", "action_reason": "reason", "previous_address": "move_import"}.get(key, "opaque")
                    child_attr = ""
                    child_mask = key == "sensitive_values"
                elif context == "change":
                    child_phase = key
                    child_mask = key in ("after_unknown", "before_sensitive", "after_sensitive")
                    child_context = "attrs" if key in ("before", "after") or child_mask else "opaque"
                    if key == "actions":
                        child_context = "actions"
                    elif key == "replace_paths":
                        child_context = "replacement_paths"
                    elif key in ("importing", "generated_config"):
                        child_context = "move_import"
                    child_attr = ""
                elif context == "attrs":
                    child_attr = key if not attr else attr + "." + key
                    child_context = "attrs" if child_attr in ATTRS.get(resource_type, {}) else "opaque"
                elif context == "expressions":
                    child_context = "expression"
                elif context == "expression":
                    child_context = "references" if key == "references" else "opaque"
                elif context == "variables":
                    child_context = "variable"
                elif context == "outputs":
                    child_context = "output"
                elif context == "output_changes":
                    child_context = "change"
                elif context == "providers":
                    child_context = "provider"
                walk(value[key], node_id, child_edge, child_context, resource_type,
                     child_attr, child_mask, depth + 1, child_phase)
        elif type(value) is list:
            item_context = {"changes": "change_resource", "resources": "resource",
                            "config_resources": "config_resource", "modules": "module",
                            "diagnostics": "diagnostic", "checks": "check"}.get(context, context)
            for index, item in enumerate(value):
                walk(item, node_id, {"index": index}, item_context, resource_type, attr, mask, depth + 1, phase)
        elif context == "actions":
            if type(value) is str and value in ACTIONS:
                node["action"] = value
            else:
                difference(node_id, "action")
        elif context == "reason":
            node["reason"] = "replace_by_request" if value == "replace_by_request" else "unrecognized_reason"
            if node["reason"] == "unrecognized_reason":
                difference(node_id, "action", "initial_actions")
        elif context == "replacement_paths":
            fields = set().union(*ATTRS.get(resource_type, {}).values())
            if type(value) is str and value in fields:
                node["path_step"] = value
            else:
                node["path_step"] = "opaque_step"
                difference(node_id, "unclassified")
        elif context == "references":
            targets = []
            if type(value) is str:
                for address in ADDRESSES:
                    if value == address:
                        targets.append((address, "resource"))
                    for field in ATTRS.get(address.split(".")[0], {}).get("", ()):
                        if value == address + "." + field:
                            targets.append((address, field))
            target, field = targets[0] if len(targets) == 1 else ("unresolved", "unresolved")
            relations.append(dict(node=node_id, relation="configuration_reference", target=target, field=field))
            if target == "unresolved":
                difference(node_id, "reference")
        return node_id

    walk(plan, None, {}, "plan")
    for address in sorted(ADDRESSES - planned_addresses):
        diffs.append(dict(node=0, category="missing", rule="required_resources", expected=address))
    sections = []
    for section in sorted(CONTEXTS["plan"]):
        present = section in plan
        sections.append(dict(section=section, presence="present" if present else "missing"))
        if section in REQUIRED:
            if not present:
                difference(0, "missing", section)
            elif kind(plan[section]) != REQUIRED[section]:
                difference(0, "type", section)
    return dict(sections=sections, resources=resources, nodes=nodes, relations=relations,
                shape_diff=diffs, coverage=dict(visited=len(nodes), complete=True, source_companions="missing", **counts))


def validate_manifest(manifest):
    """Closed output validator and forbidden-content scan, not plan approval.

    Only binding fields admit provenance identifiers. All other strings must
    occupy a schema-owned token slot; no source-derived scalar hash is allowed.
    """
    from private_plan_shape_review import binding_valid

    def keys(value, required, optional=()):
        require(type(value) is dict and set(required) <= set(value) <= set(required) | set(optional),
                "invalid_manifest")

    def integer(value, maximum=MAX_NODES):
        require(type(value) is int and 0 <= value <= maximum, "invalid_manifest")

    keys(manifest, ("schema", "mode", "binding", "sections", "resources", "nodes", "relations", "shape_diff", "coverage"))
    require(manifest["schema"] == SCHEMA and manifest["mode"] == "diagnostic-only", "invalid_manifest")
    binding_valid(manifest["binding"])
    for name in ("sections", "resources", "nodes", "relations", "shape_diff"):
        require(type(manifest[name]) is list and len(manifest[name]) <= MAX_NODES * 2, "invalid_manifest")
    nodes = manifest["nodes"]
    require(0 < len(nodes) <= MAX_NODES, "invalid_manifest")
    public_keys = set().union(*CONTEXTS.values(), VARIABLES, OUTPUTS, {"aws"},
                             *(fields for paths in ATTRS.values() for fields in paths.values()))
    classes = dict(classified=0, opaque=0, unsupported=0)
    child_counts = [0] * len(nodes)
    child_edges = [set() for _ in nodes]
    for index, node in enumerate(nodes):
        keys(node, ("id", "parent", "edge", "type", "classification"), ("cardinality", "mask", "action", "reason", "path_step"))
        integer(node["id"])
        require(node["id"] == index, "invalid_manifest")
        require(type(node["classification"]) is str and node["classification"] in classes, "invalid_manifest")
        classes[node["classification"]] += 1
        require(node["type"] in ("object", "array", "string", "number", "boolean", "null"), "invalid_manifest")
        edge = node["edge"]
        require(type(edge) is dict, "invalid_manifest")
        if index == 0:
            require(node["parent"] is None and edge == {}, "invalid_manifest")
        else:
            integer(node["parent"], index - 1)
            parent = node["parent"]
            require(nodes[parent]["type"] in ("object", "array"), "invalid_manifest")
            require(len(edge) == 1, "invalid_manifest")
            key, value = next(iter(edge.items()))
            if key == "key":
                require(type(value) is str and value in public_keys and nodes[parent]["type"] == "object", "invalid_manifest")
            elif key in ("index", "opaque_key"):
                integer(value)
                require(nodes[parent]["type"] == ("array" if key == "index" else "object"), "invalid_manifest")
            else:
                raise ShapeError("invalid_manifest")
            require((key, value) not in child_edges[parent], "invalid_manifest")
            child_edges[parent].add((key, value))
            child_counts[parent] += 1
        if node["type"] in ("object", "array"):
            integer(node.get("cardinality"))
        else:
            require("cardinality" not in node, "invalid_manifest")
        if "mask" in node:
            require(node["type"] == "boolean" and node["mask"] in ("true", "false"), "invalid_manifest")
        if "action" in node:
            require(node["type"] == "string" and type(node["action"]) is str and node["action"] in ACTIONS, "invalid_manifest")
        if "reason" in node:
            require(node["reason"] in ("replace_by_request", "unrecognized_reason"), "invalid_manifest")
        if "path_step" in node:
            require(type(node["path_step"]) is str and node["path_step"] in public_keys | {"opaque_step"}, "invalid_manifest")
    for index, node in enumerate(nodes):
        require(child_counts[index] == node.get("cardinality", 0), "invalid_manifest")
        opaque_keys = {value for key, value in child_edges[index] if key == "opaque_key"}
        require(opaque_keys == set(range(len(opaque_keys))), "invalid_manifest")
        if node["type"] == "array":
            require(child_edges[index] == {("index", i) for i in range(child_counts[index])}, "invalid_manifest")
    require(len(manifest["sections"]) == len(CONTEXTS["plan"]), "invalid_manifest")
    sections = set()
    for section in manifest["sections"]:
        keys(section, ("section", "presence"))
        require(type(section["section"]) is str and section["section"] in CONTEXTS["plan"] and
                section["section"] not in sections and section["presence"] in ("present", "missing"), "invalid_manifest")
        sections.add(section["section"])
    for resource in manifest["resources"]:
        keys(resource, ("node", "address", "type", "mode"))
        integer(resource["node"], len(nodes) - 1)
        require(type(resource["address"]) is str and resource["address"] in ADDRESSES | {"unrecognized_address"}, "invalid_manifest")
        require(type(resource["type"]) is str and resource["type"] in TYPES | {"unrecognized_type"} and
                resource["mode"] in ("managed", "data", "unrecognized_mode"), "invalid_manifest")
    for relation in manifest["relations"]:
        keys(relation, ("node", "relation", "target", "field"))
        integer(relation["node"], len(nodes) - 1)
        require(relation["relation"] == "configuration_reference" and type(relation["target"]) is str and
                relation["target"] in ADDRESSES | {"unresolved"}, "invalid_manifest")
        require(type(relation["field"]) is str and relation["field"] in public_keys | {"resource", "unresolved"}, "invalid_manifest")
    for difference in manifest["shape_diff"]:
        keys(difference, ("node", "category", "rule"), ("expected",))
        integer(difference["node"], len(nodes) - 1)
        require(difference["category"] in ("missing", "extra", "type", "cardinality", "mask", "action", "reference", "unclassified", "incomplete"), "invalid_manifest")
        require(type(difference["rule"]) is str and difference["rule"] in set(REQUIRED) | {"structure", "resource_identity", "initial_actions", "required_resources", "required_fields", "provider_schema", "move_import"}, "invalid_manifest")
        if "expected" in difference:
            require(type(difference["expected"]) is str and difference["expected"] in ADDRESSES | public_keys, "invalid_manifest")
    coverage = manifest["coverage"]
    keys(coverage, ("visited", "complete", "classified", "opaque", "unsupported", "source_companions"))
    require(coverage["source_companions"] == "missing", "invalid_manifest")
    require(coverage["complete"] is True, "inventory_incomplete")
    for field in ("visited", "classified", "opaque", "unsupported"):
        integer(coverage[field])
    require(coverage["visited"] == len(nodes) == sum(classes.values()) and
            all(coverage[key] == value for key, value in classes.items()), "invalid_manifest")
