from __future__ import annotations

from peewee import CharField, IntegerField, TextField, FloatField, Check

from api.db.base import DataBaseModel


class Memory(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    name = CharField(max_length=128, null=False, index=False, help_text="Memory name")
    avatar = TextField(null=True, help_text="avatar base64 string")
    tenant_id = CharField(max_length=32, null=False, index=True)
    memory_type = IntegerField(null=False, default=1, index=True, help_text="Bit flags (LSB->MSB): 1=raw, 2=semantic, 4=episodic, 8=procedural. E.g., 5 enables raw + episodic.")
    storage_type = CharField(
        max_length=32,
        default="table",
        null=False,
        index=True,
        choices=[("table", "Table Storage"), ("graph", "Graph Storage")],
        help_text="Storage type: table or graph"
    )
    embd_id = CharField(max_length=128, null=False, index=False, help_text="embedding model ID")
    llm_id = CharField(max_length=128, null=False, index=False, help_text="chat model ID")
    permissions = CharField(
        max_length=16,
        null=False,
        index=True,
        default="me",
        choices=[("me", "User Only"), ("team", "Team")],
        help_text="Permissions: me (user only) or team"
    )
    description = TextField(null=True, help_text="description")
    memory_size = IntegerField(default=5242880, null=False, index=False)
    forgetting_policy = CharField(
        max_length=32,
        null=False,
        default="FIFO",
        index=False,
        choices=[("LRU", "Least Recently Used"), ("FIFO", "First In First Out")],
        help_text="Forgetting policy: LRU (Least Recently Used) or FIFO (First In First Out)"
    )
    temperature = FloatField(default=0.5, index=False)
    system_prompt = TextField(null=True, help_text="system prompt", index=False)
    user_prompt = TextField(null=True, help_text="user prompt", index=False)

    class Meta:
        db_table = "memory"
        constraints = [
            Check("permissions IN ('me','team')"),
            Check("forgetting_policy IN ('LRU','FIFO')")
        ]
