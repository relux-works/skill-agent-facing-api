#!/usr/bin/env python3
"""Generate synthetic payloads in 3 formats x 4 scales for token measurement."""

import json
import os
import random
from datetime import datetime, timedelta

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))

FIELDS_FULL = ["id", "name", "status", "assignee", "description", "priority", "created", "updated"]
FIELDS_ALIAS = ["i", "n", "s", "a", "d", "p", "c", "u"]
SCALES = [5, 20, 100, 500]

# Realistic data pools
TASK_PREFIXES = [
    "Implement", "Fix", "Refactor", "Add", "Update", "Remove", "Migrate",
    "Optimize", "Configure", "Document", "Test", "Review", "Deploy",
    "Investigate", "Design", "Integrate", "Extract", "Replace", "Validate",
    "Monitor", "Automate", "Upgrade", "Restructure", "Normalize", "Cache",
]

TASK_SUBJECTS = [
    "user authentication flow", "database connection pooling",
    "API rate limiting middleware", "search indexing pipeline",
    "notification service integration", "payment webhook handler",
    "session management logic", "file upload validation",
    "error logging infrastructure", "cache invalidation strategy",
    "role-based access control", "GraphQL schema resolver",
    "CI/CD pipeline configuration", "load balancer health checks",
    "data migration scripts", "email template rendering",
    "feature flag evaluation", "metrics dashboard endpoint",
    "WebSocket reconnection logic", "batch processing queue",
    "OAuth2 token refresh flow", "image resize worker",
    "audit trail logging", "timezone handling utilities",
    "CSV export functionality", "retry policy for external calls",
    "schema validation middleware", "background job scheduler",
    "API versioning strategy", "request deduplication layer",
    "connection timeout configuration", "memory leak in worker pool",
    "pagination cursor encoding", "rate limit header propagation",
    "CORS preflight handling", "TLS certificate rotation",
    "dependency version bumps", "stale cache purge mechanism",
    "input sanitization filters", "dead letter queue processing",
]

STATUSES = ["open", "in-progress", "review", "done", "blocked"]
PRIORITIES = ["critical", "high", "medium", "low"]

ASSIGNEES = [
    "alice", "bob", "carol", "dave", "eve", "frank", "grace",
    "heidi", "ivan", "judy", "karl", "liam", "mona", "nick",
    "olivia", "pat", "quinn", "rosa", "sam", "tina",
]

DESCRIPTION_TEMPLATES = [
    "Need to {verb} the {component} to handle {scenario}. Current implementation {problem}.",
    "The {component} has issues when {scenario}. We should {verb} it to {goal}.",
    "{component} needs attention: {scenario} causes {problem}. Plan: {verb} and {goal}.",
    "As discussed in standup, {verb} {component}. {scenario} is blocking {goal}.",
    "Follow-up from incident: {component} failed during {scenario}. Must {verb} to prevent {problem}.",
]

DESC_VERBS = ["refactor", "rewrite", "patch", "extend", "simplify", "harden", "decouple", "wrap"]
DESC_COMPONENTS = [
    "the auth module", "our caching layer", "the API gateway",
    "the worker pool", "the event bus", "the ORM layer",
    "the config loader", "the middleware stack",
]
DESC_SCENARIOS = [
    "high traffic spikes", "concurrent writes", "large payloads",
    "network partitions", "cold starts", "schema changes",
    "token expiration", "failover events",
]
DESC_PROBLEMS = [
    "silently drops requests", "leaks memory over time",
    "returns stale data", "times out under load",
    "fails without retry", "corrupts state on restart",
]
DESC_GOALS = [
    "improve reliability", "reduce latency by 40%",
    "support horizontal scaling", "meet SLA requirements",
    "unblock the frontend team", "pass the security audit",
]


def generate_description():
    tmpl = random.choice(DESCRIPTION_TEMPLATES)
    return tmpl.format(
        verb=random.choice(DESC_VERBS),
        component=random.choice(DESC_COMPONENTS),
        scenario=random.choice(DESC_SCENARIOS),
        problem=random.choice(DESC_PROBLEMS),
        goal=random.choice(DESC_GOALS),
    )


def generate_task_name():
    return f"{random.choice(TASK_PREFIXES)} {random.choice(TASK_SUBJECTS)}"


def random_date(start_year=2025, end_year=2026):
    start = datetime(start_year, 1, 1)
    end = datetime(end_year, 2, 12)
    delta = end - start
    rand_days = random.randint(0, delta.days)
    d = start + timedelta(days=rand_days)
    return d.strftime("%Y-%m-%d")


def generate_items(n):
    items = []
    for i in range(1, n + 1):
        created = random_date()
        # updated is same or after created
        created_dt = datetime.strptime(created, "%Y-%m-%d")
        days_after = random.randint(0, 30)
        updated_dt = min(created_dt + timedelta(days=days_after), datetime(2026, 2, 12))
        items.append({
            "id": f"TASK-{i:04d}",
            "name": generate_task_name(),
            "status": random.choice(STATUSES),
            "assignee": random.choice(ASSIGNEES),
            "description": generate_description(),
            "priority": random.choice(PRIORITIES),
            "created": created,
            "updated": updated_dt.strftime("%Y-%m-%d"),
        })
    return items


def to_json(items):
    """Standard JSON array."""
    return json.dumps(items, indent=2)


def to_compact_full(items):
    """CSV-style with full field names as header."""
    header = ",".join(FIELDS_FULL)
    rows = []
    for item in items:
        row_vals = []
        for f in FIELDS_FULL:
            val = str(item[f])
            # Quote if contains comma
            if "," in val:
                val = f'"{val}"'
            row_vals.append(val)
        rows.append(",".join(row_vals))
    return header + "\n" + "\n".join(rows)


def to_compact_alias(items):
    """CSV-style with abbreviated field names as header."""
    header = ",".join(FIELDS_ALIAS)
    rows = []
    for item in items:
        row_vals = []
        for f in FIELDS_FULL:
            val = str(item[f])
            if "," in val:
                val = f'"{val}"'
            row_vals.append(val)
        rows.append(",".join(row_vals))
    return header + "\n" + "\n".join(rows)


def main():
    random.seed(42)  # Reproducible

    for scale in SCALES:
        items = generate_items(scale)

        # JSON variant
        json_path = os.path.join(SCRIPT_DIR, f"json-{scale}.txt")
        with open(json_path, "w") as f:
            f.write(to_json(items))

        # Compact full field names
        compact_full_path = os.path.join(SCRIPT_DIR, f"compact-full-{scale}.txt")
        with open(compact_full_path, "w") as f:
            f.write(to_compact_full(items))

        # Compact aliased field names
        compact_alias_path = os.path.join(SCRIPT_DIR, f"compact-alias-{scale}.txt")
        with open(compact_alias_path, "w") as f:
            f.write(to_compact_alias(items))

        print(f"Generated scale={scale}: json-{scale}.txt, compact-full-{scale}.txt, compact-alias-{scale}.txt")


if __name__ == "__main__":
    main()
