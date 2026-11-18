#!/bin/bash
set -e

# 02_bcompat.sh — Vastbase B-mode compatibility options
# Must be run as the vastbase/vb user (who owns $PGDATA)
# The image's entrypoint passes VB_DBCOMPATIBILITY to vb_initdb on first
# initialization, which fixes the instance-wide compatibility mode. The
# GUCs below are B-mode behavior tweaks and must not run on other modes.

VB_MODE="${VB_DBCOMPATIBILITY:-B}"

if [ "${VB_MODE}" != "B" ]; then
    echo "VB_DBCOMPATIBILITY=${VB_MODE}: skipping B-mode compatibility options."
    exit 0
fi

echo "Setting B-mode compatibility options..."
gs_guc reload -D "$PGDATA" -c "b_format_behavior_compat_options = 'set_keyword_as_colname, show_attalias_as_colname, pg_todate_format'"
echo "Done. Compatibility options applied."
