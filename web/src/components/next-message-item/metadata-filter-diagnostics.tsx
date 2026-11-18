import { IMetadataFilterDiagnostic } from '@/interfaces/database/chat';
import { Fragment } from 'react';
import { useTranslation } from 'react-i18next';

const FilterStatusKeys: Record<string, string> = {
  applied: 'chat.metadataFilterApplied',
  disabled: 'chat.metadataFilterDisabled',
  no_matches: 'chat.metadataFilterNoMatches',
  not_generated: 'chat.metadataFilterNotGenerated',
  unsupported: 'chat.metadataFilterUnsupported',
};

function formatFilterValue(value: unknown) {
  if (typeof value === 'string' || typeof value === 'number') {
    return String(value);
  }
  return JSON.stringify(value);
}

function MetadataFilterEntry({
  diagnostic,
}: {
  diagnostic: IMetadataFilterDiagnostic;
}) {
  const { t } = useTranslation();
  const statusKey = FilterStatusKeys[diagnostic.status];
  const status = statusKey ? t(statusKey) : diagnostic.status;
  const logic = diagnostic.logic === 'or' ? 'or' : 'and';

  return (
    <div className="space-y-1">
      <div className="break-words text-text-secondary">
        {diagnostic.tool_name && <span>{diagnostic.tool_name}: </span>}
        <span>{status}</span>
      </div>
      {!!diagnostic.conditions?.length && (
        <ul className="list-disc pl-5 text-text-primary">
          {diagnostic.conditions.map((condition, index) => (
            <Fragment key={`${condition.key}-${condition.op}-${index}`}>
              {index > 0 && (
                <li className="list-none text-text-secondary">
                  {t(`chat.metadataFilterLogic.${logic}`)}
                </li>
              )}
              <li className="break-words">
                {condition.key} {condition.op}{' '}
                {formatFilterValue(condition.value)}
              </li>
            </Fragment>
          ))}
        </ul>
      )}
      {diagnostic.status === 'applied' && (
        <div className="text-text-secondary">
          {t('chat.metadataFilterMatchedDocuments', {
            count: diagnostic.matched_document_count,
          })}
        </div>
      )}
    </div>
  );
}

export function MetadataFilterDiagnostics({
  diagnostics,
}: {
  diagnostics?: IMetadataFilterDiagnostic[];
}) {
  const { t } = useTranslation();

  return (
    <section className="mt-3 rounded-md border border-border-button bg-bg-card p-3 text-xs">
      <div className="mb-2 font-medium text-text-primary">
        {t('chat.metadataFilter')}
      </div>
      {diagnostics && diagnostics.length > 0 ? (
        <div className="space-y-3">
          {diagnostics.map((diagnostic, index) => (
            <MetadataFilterEntry
              key={`${diagnostic.tool_name ?? diagnostic.method}-${index}`}
              diagnostic={diagnostic}
            />
          ))}
        </div>
      ) : (
        <div className="text-text-secondary">
          {t('chat.metadataFilterUnavailable')}
        </div>
      )}
    </section>
  );
}
