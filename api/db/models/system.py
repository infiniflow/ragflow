from __future__ import annotations

from peewee import CharField

from api.db.base import DataBaseModel


class SystemSettings(DataBaseModel):
    name = CharField(max_length=128, primary_key=True)
    source = CharField(max_length=32, null=False, index=False)
    data_type = CharField(max_length=32, null=False, index=False)
    value = CharField(max_length=1024, null=False, index=False)

    class Meta:
        db_table = "system_settings"
