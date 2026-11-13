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
 * A declared class, as the filter offers it.
 *
 * `entities` is optional because the call sites that only have a type name (the
 * instances page) still pass the closed set; when the count is absent the class
 * bubble falls back to the smallest radius instead of inventing one.
 */
export interface AboxClassOption {
  type: string;
  label?: string;
  entities?: number;
  declared?: boolean;
}

/** A declared object property; a polymorphic one appears once per edge. */
export interface AboxPropertyOption {
  type: string;
  label?: string;
  source: string;
  target: string;
  relations?: number;
  declared?: boolean;
}

/** Which half of the ABox a row belongs to. */
export type AboxRowKind = 'entity' | 'relation';

/** The two canvases: one bubble per class, or the instances of one class. */
export type AboxGraphView = 'classes' | 'instances';

/**
 * The four ways to read the same rows.
 *
 * `instances` is the canvas (bounded, cannot page) and `entities` is the same
 * class as rows (paged, can reach the last one). They are separate tabs rather
 * than a toggle because the difference is the point: the canvas says how much
 * it left out and points here.
 */
export type AboxTab = AboxGraphView | 'entities' | 'relations';
