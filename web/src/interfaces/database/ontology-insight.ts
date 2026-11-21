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
 * The measured half of a model, as the workspace reads it.
 *
 * Every number here was produced by a compile that already finished. Nothing on
 * this shape is computed while a page is open — that is the whole reason it is
 * stored — so a reader can always point at "as of this compile" instead of
 * wondering whether the figure moved while they were looking at it.
 */
export interface IOntologyInsightField {
  type: string;
  assertions: number;
  instances: number;
  /** Ratio, not a percentage: the UI formats it. */
  fill_rate: number;
  required?: boolean;
  missing?: number;
  inherited_from?: string;
}

export interface IOntologyInsightClass {
  type: string;
  label?: string;
  description?: string;
  entities: number;
  /** False for a class the corpus produced that the model never declared. */
  declared: boolean;
  fields?: IOntologyInsightField[];
}

export interface IOntologyInsightProperty {
  type: string;
  label?: string;
  source: string;
  target: string;
}

export interface IOntologyDrift {
  undeclared_classes: IOntologyInsightClass[];
  unused_classes: IOntologyInsightClass[];
  unused_properties: IOntologyInsightProperty[];
  classes: IOntologyInsightClass[];
  total_entities: number;
  total_relations: number;
  dropped_relations: number;
  untyped_entities: number;
  /** The count scan hit its cap, so every number is a lower bound. */
  truncated: boolean;
}

export type OntologyCompetencyElementKind = 'class' | 'property' | 'relation';

export interface IOntologyCompetencyElement {
  kind: OntologyCompetencyElementKind;
  /** `class`, `class.field`, or `from->to` — the same spelling the model uses. */
  name: string;
  required: boolean;
  /** Declared AND exercised. A declared element with no rows is not satisfied. */
  satisfied: boolean;
  observed: number;
  declared: boolean;
  /** `undeclared` | `not_extracted`, translated by the UI. */
  reason?: string;
}

export interface IOntologyCompetencyCoverage {
  id: string;
  question: string;
  required: number;
  satisfied: number;
  answerable: boolean;
  elements: IOntologyCompetencyElement[];
}

export interface IOntologyInsightPayload {
  drift: IOntologyDrift;
  questions: IOntologyCompetencyCoverage[];
  required: number;
  satisfied: number;
}

export interface IOntologyInsight {
  template_id: string;
  dataset_id: string;
  /** The model version the measured rows were compiled under. */
  version: number;
  as_of: number;
  /** The model has been published since; the numbers describe the old one. */
  stale: boolean;
  /**
   * The working config differs from the version these numbers were compiled
   * under. Compilation reads the published snapshot, so an unpublished edit
   * changes nothing — including after recompiling.
   */
  draft_pending: boolean;
  payload: IOntologyInsightPayload;
}

export interface IOntologyContractExport {
  template_id: string;
  name: string;
  version: number;
  publish_status: string;
  published_at?: string;
  published_by?: string;
  markdown: string;
  machine: string;
  schema: Record<string, unknown>;
  coverage_note?: string;
  drift_note?: string;
}

/**
 * One competency question as the model declares it. The wire form is the flat
 * `competency.fields` section of the template config; this is the editor's view
 * of the same data.
 */
export interface IOntologyCompetencyQuestionDraft {
  type: string;
  question: string;
  /** `class:contract | relation:contract->vendor | ?class:person` */
  elements: string;
}
