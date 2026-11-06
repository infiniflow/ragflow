'use client';

import { OntologyAssertionRow } from '@/components/ontology-assertion-row';
import { OntologyEvidenceBadge } from '@/components/ontology-evidence-badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  IOntologyPreviewOutcome,
  useOntologyPreview,
} from '@/hooks/use-ontology-request';
import { ontologyEvidenceHint } from '@/utils/ontology-evidence';
import { X } from 'lucide-react';
import { ChangeEvent, useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';

type OntologyPreviewProps = {
  modelId?: string;
  modelVersion?: number;
  knowledgeDatasetIds?: string[];
};

/**
 * Runs the knowledge layer for a typed question and shows what it answered.
 *
 * It exists because the two things that decide whether this layer is useful are
 * facts about the corpus, not about the form: whether the model narrows anything
 * here, and whether its quotations resolve to documents the assistant may read.
 * Both are visible only by searching, and a preview searches before the setting
 * is saved.
 */
export function OntologyPreview({
  modelId,
  modelVersion,
  knowledgeDatasetIds,
}: OntologyPreviewProps) {
  const { t } = useTranslation();
  const [question, setQuestion] = useState('');
  const { loading, outcome, runPreview, clearPreview } = useOntologyPreview();

  const bound = Boolean(modelId) && Boolean(knowledgeDatasetIds?.length);
  const canRun = bound && question.trim().length > 0 && !loading;

  const handleQuestionChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => setQuestion(event.target.value),
    [],
  );

  const handleRun = useCallback(() => {
    if (!canRun) {
      return;
    }
    void runPreview({
      dataset_ids: knowledgeDatasetIds ?? [],
      question: question.trim(),
      ontology_template_id: modelId,
      ontology_version: modelVersion ?? 0,
    });
  }, [
    canRun,
    knowledgeDatasetIds,
    modelId,
    modelVersion,
    question,
    runPreview,
  ]);

  const handleClear = useCallback(() => clearPreview(), [clearPreview]);

  return (
    <div className="space-y-3 rounded-lg border border-border-button bg-bg-card p-3">
      <p className="text-sm text-text-primary">
        {t('chat.knowledgeLayerPreview')}
      </p>
      <p className="text-xs text-text-secondary">
        {t('chat.knowledgeLayerPreviewTip')}
      </p>
      <div className="flex items-center gap-2">
        <Input
          value={question}
          onChange={handleQuestionChange}
          placeholder={t('chat.knowledgeLayerPreviewQuestion')}
          className="h-8"
        />
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={!canRun}
          onClick={handleRun}
        >
          {loading
            ? t('chat.knowledgeLayerPreviewRunning')
            : t('chat.knowledgeLayerPreviewRun')}
        </Button>
      </div>
      {!bound && (
        <p className="text-xs text-text-disabled">
          {t('chat.knowledgeLayerPreviewNeedsBinding')}
        </p>
      )}
      {outcome && <OntologyReceipt outcome={outcome} onClear={handleClear} />}
    </div>
  );
}

function OntologyReceipt({
  outcome,
  onClear,
}: {
  outcome: IOntologyPreviewOutcome;
  onClear: () => void;
}) {
  const { t } = useTranslation();
  const result = outcome.result;

  if (!outcome.ok || !result) {
    return (
      <p className="rounded-md border border-border-button p-2 text-xs text-text-secondary">
        {outcome.message ?? t('chat.ontologyPreviewFailed')}
      </p>
    );
  }

  const digest = result.digest;
  const assertions = digest?.assertions ?? [];
  const missingClasses = digest?.coverage?.missing_classes ?? [];
  const suggested = result.suggested_constraints;

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <OntologyEvidenceBadge level={result.evidence_level} />
        <span className="text-text-primary">
          {t('chat.ontologyPreviewMatched', { count: result.total })}
        </span>
        <span className="text-text-disabled">
          {t('chat.ontologyPreviewModel', {
            name: result.model?.name || result.model?.id,
            version: result.model?.version ?? 0,
          })}
        </span>
        <Button
          type="button"
          variant="transparent"
          size="icon-sm"
          className="ml-auto"
          onClick={onClear}
        >
          <X className="size-4" />
        </Button>
      </div>
      <p className="text-xs text-text-secondary">
        {ontologyEvidenceHint(result.evidence_level, t)}
      </p>
      {digest?.degrade && (
        <p className="text-xs text-text-secondary">
          {t('chat.ontologyPreviewDegrade', {
            code: digest.degrade.code,
            reason: digest.degrade.reason,
          })}
        </p>
      )}
      {Boolean(result.filter) && (
        <code className="block overflow-x-auto rounded-md bg-bg-input p-2 text-xs text-text-secondary">
          {JSON.stringify(result.filter)}
        </code>
      )}
      {digest && (
        <p className="text-xs text-text-disabled">
          {t('chat.ontologyPreviewCitations', {
            resolvable: digest.citations.resolvable,
            unresolvable: digest.citations.unresolvable,
          })}
        </p>
      )}
      {Boolean(digest?.unreviewed_count) && (
        <p className="text-xs text-text-disabled">
          {t('chat.ontologyPreviewUnreviewed', {
            count: digest?.unreviewed_count ?? 0,
          })}
        </p>
      )}
      {Boolean(missingClasses.length) && (
        <p className="text-xs text-text-disabled">
          {t('chat.ontologyPreviewMissingClasses', {
            classes: missingClasses.join(', '),
          })}
        </p>
      )}
      {digest?.coverage?.truncated && (
        <p className="text-xs text-text-disabled">
          {t('chat.ontologyPreviewCoverageTruncated', {
            sampled: digest.coverage.sampled,
            limit: digest.coverage.sample_limit,
          })}
        </p>
      )}
      {Boolean(
        suggested?.entity_types?.length ||
        suggested?.properties?.length ||
        suggested?.from_types?.length ||
        suggested?.to_types?.length,
      ) && (
        <p className="text-xs text-text-disabled">
          {t('chat.ontologyPreviewSuggested', {
            constraints: JSON.stringify(suggested),
          })}
        </p>
      )}
      {assertions.length === 0 ? (
        <p className="text-xs text-text-secondary">
          {t('chat.ontologyPreviewNoAssertion')}
        </p>
      ) : (
        <ul className="space-y-2">
          {assertions.map((assertion, index) => (
            <OntologyAssertionRow
              key={`${assertion.kind}-${assertion.type}-${assertion.name}-${assertion.from}-${assertion.to}-${index}`}
              assertion={assertion}
            />
          ))}
        </ul>
      )}
    </div>
  );
}
