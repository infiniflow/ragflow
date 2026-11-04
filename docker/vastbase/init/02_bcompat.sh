#!/bin/bash
set -e

# 02_bcompat.sh — Set Vastbase B-mode compatibility options
# Must be run as the vastbase/vb user (who owns $PGDATA)

echo "Setting B-mode compatibility options..."
gs_guc reload -D "$PGDATA" -c "b_format_behavior_compat_options = 'set_keyword_as_colname, show_attalias_as_colname, pg_todate_format'"
echo "Done. Compatibility options applied."
