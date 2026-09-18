#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

import json
from datetime import datetime

from peewee import IntegrityError

from api.db.db_models import DB, PipelineDSLVersion
from common.time_utils import current_timestamp, datetime_format


MAX_PIPELINE_DSL_VERSION_INSERT_ATTEMPTS = 16


class PipelineDSLVersionService:
    """Persist immutable, monotonically versioned pipeline DSL definitions."""

    model = PipelineDSLVersion

    @staticmethod
    def _normalize_dsl(dsl):
        if not isinstance(dsl, dict):
            raise ValueError("Pipeline DSL must be a JSON object.")
        try:
            return json.loads(json.dumps(dsl, ensure_ascii=False))
        except (TypeError, ValueError) as exc:
            raise ValueError("Pipeline DSL must be JSON-serializable.") from exc

    @classmethod
    def _get_latest(cls, dsl_id):
        return (
            cls.model.select()
            .where(cls.model.dsl_id == dsl_id)
            .order_by(cls.model.version.desc())
            .first()
        )

    @classmethod
    @DB.connection_context()
    def get_or_create(cls, dsl_id, dsl):
        dsl_id = str(dsl_id or "").strip()
        if not dsl_id:
            raise ValueError("Pipeline DSL ID is required.")
        normalized = cls._normalize_dsl(dsl)

        for _ in range(MAX_PIPELINE_DSL_VERSION_INSERT_ATTEMPTS):
            latest = cls._get_latest(dsl_id)
            if latest is not None and latest.dsl == normalized:
                return latest

            next_version = 1 if latest is None else latest.version + 1
            timestamp = current_timestamp()
            now = datetime_format(datetime.now())  # noqa: DTZ005
            try:
                # A nested atomic block becomes a savepoint when the caller is
                # already in a transaction. PostgreSQL can therefore recover
                # from a racing composite-key conflict without aborting the
                # caller's whole transaction.
                with DB.atomic():
                    return cls.model.create(
                        dsl_id=dsl_id,
                        version=next_version,
                        dsl=normalized,
                        create_time=timestamp,
                        create_date=now,
                        update_time=timestamp,
                        update_date=now,
                    )
            except IntegrityError:
                # Another writer inserted this version after our read. Re-read
                # the head: identical DSL will be reused, while a different DSL
                # advances once more.
                continue

        raise RuntimeError(
            f"Could not store pipeline DSL version for {dsl_id!r} after "
            f"{MAX_PIPELINE_DSL_VERSION_INSERT_ATTEMPTS} attempts due to concurrent updates."
        )
