import { Collapse } from '@/components/collapse';
import CopyToClipboard from '@/components/copy-to-clipboard';
import { IModelInputSnapshot } from '@/interfaces/database/agent';
import { useTranslation } from 'react-i18next';
import JsonView from 'react18-json-view';
import styles from './model-inputs.module.css';

/** Displays captured attempts without rerunning or reconstructing model inputs. */
export function ModelInputs({ snapshot }: { snapshot?: IModelInputSnapshot }) {
  const { t } = useTranslation();
  return (
    <div className="rounded-md border p-3">
      <Collapse title={t('flow.modelInputs')}>
        {snapshot ? (
          <div className="space-y-2">
            <div className="flex items-center justify-between gap-3">
              <p className="text-sm text-text-secondary">
                {t('flow.modelInputSummary', {
                  count: snapshot.calls.length,
                  omittedCount: snapshot.omitted_count,
                })}
              </p>
              <CopyToClipboard
                text={JSON.stringify(snapshot, null, 2)}
                aria-label={t('common.copy')}
              />
            </div>
            {snapshot.calls.length === 0 && snapshot.omitted_count === 0 && (
              <p className="text-sm text-text-secondary">
                {t('flow.noModelInputs')}
              </p>
            )}
            <JsonView
              src={snapshot}
              displaySize
              collapseStringsAfterLength={100000000000}
              className={`${styles.modelInputs} max-h-[400px] overflow-auto break-words bg-muted p-2`}
            />
          </div>
        ) : (
          <p className="text-sm text-text-secondary">
            {t('flow.modelInputsUnavailable')}
          </p>
        )}
      </Collapse>
    </div>
  );
}
