#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

import logging

logger = logging.getLogger(__name__)

# beartype's runtime type-checking is a development aid, not a hard runtime
# requirement. Tolerate a missing install (e.g. partial / slim builds) instead
# of failing to import the package — fixes #14931.
try:
    from beartype.claw import beartype_this_package
    beartype_this_package()
except ImportError:
    logger.warning(
        "beartype is not installed; deepdoc will run without "
        "beartype-based runtime type checks (beartype.claw.beartype_this_package "
        "skipped)."
    )
