/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

/**
 * Enterprise seam for the reasoning-engine options.
 *
 * Upstream ships this as an empty list: the open-source build only offers the
 * naive ('0') and react ('5') levels, which the shared selector declares itself.
 * The enterprise repository fills the same path with the agentic pro option.
 */
import type { ThinkingOption } from '@/hooks/use-thinking-level';

export const extraThinkingOptions: ThinkingOption[] = [];
