"""Deterministic safety gate. An LLM cannot override deployment policy."""
REQUIRED_TOOLS = {"get_release_changes", "get_ci_status", "get_release_policy", "get_known_risks"}

def evaluate(evidence: dict) -> tuple[str, list[str]]:
    if set(evidence) != REQUIRED_TOOLS:
        raise ValueError("All four MCP evidence sources are required")
    changes = evidence["get_release_changes"]
    ci = evidence["get_ci_status"]
    policy = evidence["get_release_policy"]
    risks = evidence["get_known_risks"]
    if changes["service"] != policy["service"] or len({changes["release"], ci["release"], risks["release"]}) != 1:
        raise ValueError("Evidence identity mismatch")
    blockers = []
    for check in policy["required_checks"]:
        if ci["checks"].get(check) != "passed":
            blockers.append(f"Required CI check {check} is not passed")
    if ci["coverage_percent"] < policy["minimum_coverage_percent"]:
        blockers.append(f"Coverage {ci['coverage_percent']}% is below {policy['minimum_coverage_percent']}%")
    if changes["migration"] and policy["require_migration_rollback"] and not changes["rollback_documented"]:
        blockers.append("Database migration has no documented rollback")
    for risk in risks["risks"]:
        if not risk["resolved"] and risk["severity"] in policy["blocking_severities"]:
            blockers.append(f"Unresolved {risk['severity']} risk {risk['id']}: {risk['description']}")
    return ("blocked" if blockers else "ready"), blockers
