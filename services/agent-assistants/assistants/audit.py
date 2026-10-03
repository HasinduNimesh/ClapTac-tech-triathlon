"""Structured audit log, same shape as pkg/audit LogPublisher.

Only bounded facts are recorded (counts, card types, hashes). The raw order
text and bearer tokens are never logged.
"""

import hashlib
import json
import logging
import uuid
from datetime import datetime, timezone

logger = logging.getLogger("agent-assistants")

ACTION_ORDER_DRAFTED = "AGENT_ORDER_DRAFTED"
ACTION_DASHBOARD_DRAFTED = "AGENT_DASHBOARD_DRAFTED"
ACTION_AGENT_FAILED = "AGENT_TOOL_FAILED"


def text_hash(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


def publish(action: str, actor_id: str, correlation_id: str, new_state: dict, resource_id: str = "") -> None:
    event = {
        "msg": "audit_event",
        "event_id": str(uuid.uuid4()),
        "correlation_id": correlation_id,
        "actor_id": actor_id,
        "actor_type": "human",
        "action": action,
        "resource_type": "agent_action",
        "resource_id": resource_id,
        "new_state": new_state,
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "source": "agent-assistants",
    }
    logger.info(json.dumps(event, separators=(",", ":"), default=str))
