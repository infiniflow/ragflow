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
# distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

"""
Sandbox providers package.

This package contains:
- base.py: Base interface for all sandbox providers
- manager.py: Provider manager for managing active provider
- self_managed.py: Self-managed provider implementation (wraps existing executor_manager)
- aliyun_codeinterpreter.py: Aliyun Code Interpreter provider implementation
  Official Documentation: https://help.aliyun.com/zh/functioncompute/fc/sandbox-sandbox-code-interepreter
- e2b.py: E2B provider implementation
"""

from .base import SandboxProvider, SandboxInstance, ExecutionResult
from .manager import ProviderManager
from .self_managed import SelfManagedProvider
from .aliyun_codeinterpreter import AliyunCodeInterpreterProvider
from .e2b import E2BProvider

__all__ = [
    "SandboxProvider",
    "SandboxInstance",
    "ExecutionResult",
    "ProviderManager",
    "SelfManagedProvider",
    "AliyunCodeInterpreterProvider",
    "E2BProvider",
]
