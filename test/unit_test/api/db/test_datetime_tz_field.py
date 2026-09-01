"""
Tests for DateTimeTzField conversion between database driver values and
Python datetimes.

The sync_logs poll_range columns are declared VARCHAR, but deployments
upgraded from older schemas can hold them in native DATETIME columns, so the
driver may hand back datetime objects (or zero-date strings) instead of ISO
strings. Conversion must never raise: one bad value previously wedged the
whole sync-task scheduler with "list index out of range".
"""

from datetime import UTC, datetime

from api.db.db_models import DateTimeTzField


class TestDateTimeTzFieldPythonValue:
    def test_none(self):
        assert DateTimeTzField().python_value(None) is None

    def test_iso_string_with_timezone(self):
        dt = DateTimeTzField().python_value("2026-08-14T10:00:00.123456+00:00")
        assert dt == datetime(2026, 8, 14, 10, 0, 0, 123456, tzinfo=UTC)

    def test_iso_string_without_timezone_assumes_utc(self):
        dt = DateTimeTzField().python_value("2026-08-14 10:00:00")
        assert dt == datetime(2026, 8, 14, 10, 0, tzinfo=UTC)

    def test_native_datetime_returned_by_driver(self):
        naive = datetime(2026, 8, 14, 10, 0, 0, 123000, tzinfo=UTC).replace(tzinfo=None)
        dt = DateTimeTzField().python_value(naive)
        assert dt == datetime(2026, 8, 14, 10, 0, 0, 123000, tzinfo=UTC)

    def test_native_aware_datetime_kept(self):
        aware = datetime(2026, 8, 14, 10, 0, tzinfo=UTC)
        assert DateTimeTzField().python_value(aware) is aware

    def test_zero_date_string_falls_back_to_none(self):
        assert DateTimeTzField().python_value("0000-00-00 00:00:00") is None

    def test_garbage_string_falls_back_to_none(self):
        assert DateTimeTzField().python_value("not-a-date") is None

    def test_empty_string_falls_back_to_none(self):
        assert DateTimeTzField().python_value("") is None


class TestDateTimeTzFieldDbValue:
    def test_none(self):
        assert DateTimeTzField().db_value(None) is None

    def test_naive_datetime_written_as_utc_iso(self):
        naive = datetime(2026, 8, 14, 10, 0, tzinfo=UTC).replace(tzinfo=None)
        assert DateTimeTzField().db_value(naive) == "2026-08-14T10:00:00+00:00"

    def test_aware_datetime_written_as_iso(self):
        assert DateTimeTzField().db_value(datetime(2026, 8, 14, 10, 0, tzinfo=UTC)) == "2026-08-14T10:00:00+00:00"

    def test_db_value_round_trips_through_python_value(self):
        field = DateTimeTzField()
        stored = field.db_value(datetime(2026, 8, 14, 10, 0, 0, 123456, tzinfo=UTC))
        assert field.python_value(stored) == datetime(2026, 8, 14, 10, 0, 0, 123456, tzinfo=UTC)
