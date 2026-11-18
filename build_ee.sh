#!/usr/bin/env bash
# EE (enterprise) build extensions, sourced by build.sh.
#
# This file is an intentionally empty placeholder on the OSS/upstream path. The
# EE build (ragflow_enterprise) populates it with enterprise-only build/test
# logic (e.g. run_ee_native_integration_tests). build.sh only sources this file
# when present and guards the hook call, so an empty/absent file is a harmless
# no-op.
#
# Keep this file free of executable side effects at source time: it must only
# DEFINE functions.
