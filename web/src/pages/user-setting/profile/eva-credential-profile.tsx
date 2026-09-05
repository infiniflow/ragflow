import PasswordInput from '@/components/originui/password-input';
import { Button } from '@/components/ui/button';
import message from '@/components/ui/message';
import { Modal } from '@/components/ui/modal/modal';
import { useTranslate } from '@/hooks/common-hooks';
import {
  deleteEvaUserCredential,
  EvaUserCredentialStatus,
  listEvaUserCredentials,
  putEvaUserCredential,
} from '@/services/user-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { KeyRound, Loader2Icon, Trash2 } from 'lucide-react';
import { useState } from 'react';

const queryKey = ['eva-user-credentials'];

export function EvaCredentialProfile() {
  const { t } = useTranslate('setting');
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<EvaUserCredentialStatus>();
  const [removing, setRemoving] = useState<EvaUserCredentialStatus>();
  const [token, setToken] = useState('');

  const credentialsQuery = useQuery<EvaUserCredentialStatus[]>({
    queryKey,
    queryFn: async () => {
      const { data } = await listEvaUserCredentials();
      if (data.code !== 0)
        throw new Error(data.message || t('evaTokenLoadFailed'));
      return data.data?.items ?? [];
    },
  });
  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!editing) return;
      const { data } = await putEvaUserCredential(editing.connector_id, token);
      if (data.code !== 0)
        throw new Error(data.message || t('evaTokenSaveFailed'));
    },
    onSuccess: async () => {
      setEditing(undefined);
      setToken('');
      await queryClient.invalidateQueries({ queryKey });
      message.success(t('evaTokenSaved'));
    },
  });
  const deleteMutation = useMutation({
    mutationFn: async () => {
      if (!removing) return;
      const { data } = await deleteEvaUserCredential(removing.connector_id);
      if (data.code !== 0)
        throw new Error(data.message || t('evaTokenDeleteFailed'));
    },
    onSuccess: async () => {
      setRemoving(undefined);
      await queryClient.invalidateQueries({ queryKey });
      message.success(t('evaTokenDeleted'));
    },
  });

  const items = credentialsQuery.data ?? [];
  return (
    <section data-testid="settings-eva-profile">
      <div className="flex items-start gap-4">
        <label className="w-[190px] text-sm font-medium">
          {t('evaApiToken')}
        </label>
        <div className="flex-1 space-y-3">
          {credentialsQuery.isLoading && (
            <div className="flex items-center gap-2 text-sm text-text-secondary">
              <Loader2Icon className="size-4 animate-spin" />
              {t('evaTokenLoading')}
            </div>
          )}
          {!credentialsQuery.isLoading && items.length === 0 && (
            <p className="text-sm text-text-secondary">
              {t('evaTokenNoConnectors')}
            </p>
          )}
          {items.map((item) => (
            <div
              key={item.scope}
              className="flex flex-wrap items-center gap-3 rounded-md border border-border-button px-3 py-2"
            >
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm text-text-primary">
                  {item.scope}
                </div>
                <div className="text-xs text-text-secondary">
                  {item.configured
                    ? t('evaTokenConfigured')
                    : t('evaTokenNotConfigured')}
                </div>
              </div>
              <Button
                type="button"
                variant="outline"
                data-testid={`eva-token-edit-${item.connector_id}`}
                onClick={() => {
                  setEditing(item);
                  setToken('');
                }}
              >
                <KeyRound className="size-3.5" />
                {item.configured
                  ? t('evaTokenReplaceAction')
                  : t('evaTokenAddAction')}
              </Button>
              {item.configured && (
                <Button
                  type="button"
                  variant="ghost"
                  data-testid={`eva-token-delete-${item.connector_id}`}
                  onClick={() => setRemoving(item)}
                  aria-label={t('evaTokenDelete')}
                >
                  <Trash2 className="size-3.5" />
                </Button>
              )}
            </div>
          ))}
          <p className="text-xs leading-5 text-text-secondary">
            {t('evaTokenHowToGet')}
          </p>
          <p className="text-xs leading-5 text-text-secondary">
            {t('evaTokenUsageDescription')}
          </p>
          {credentialsQuery.error && (
            <p className="text-xs text-state-error" role="alert">
              {credentialsQuery.error.message}
            </p>
          )}
        </div>
      </div>

      <Modal
        open={Boolean(editing)}
        testId="eva-token-edit-modal"
        okButtonTestId="eva-token-save"
        title={editing?.configured ? t('evaTokenReplace') : t('evaTokenAdd')}
        maskClosable={false}
        confirmLoading={saveMutation.isPending}
        disabled={!token.trim()}
        okText={t('save', { keyPrefix: 'common' })}
        onOk={() => saveMutation.mutate()}
        onCancel={() => {
          setEditing(undefined);
          setToken('');
          saveMutation.reset();
        }}
        onOpenChange={(open) => {
          if (!open) {
            setEditing(undefined);
            setToken('');
            saveMutation.reset();
          }
        }}
      >
        <div className="space-y-3 py-4">
          <p className="text-sm text-text-secondary">{editing?.scope}</p>
          <PasswordInput
            value={token}
            data-testid="eva-token-input"
            maxLength={4096}
            autoComplete="new-password"
            placeholder={t('evaApiToken')}
            onChange={(event) => setToken(event.target.value)}
          />
          {saveMutation.error && (
            <p className="text-sm text-state-error" role="alert">
              {saveMutation.error.message}
            </p>
          )}
        </div>
      </Modal>

      <Modal
        open={Boolean(removing)}
        testId="eva-token-delete-modal"
        okButtonTestId="eva-token-delete-confirm"
        title={t('evaTokenDelete')}
        type="warning"
        confirmLoading={deleteMutation.isPending}
        okText={t('delete', { keyPrefix: 'common' })}
        onOk={() => deleteMutation.mutate()}
        onCancel={() => {
          setRemoving(undefined);
          deleteMutation.reset();
        }}
        onOpenChange={(open) => {
          if (!open) {
            setRemoving(undefined);
            deleteMutation.reset();
          }
        }}
      >
        <p className="py-4 text-sm text-text-secondary">
          {t('evaTokenDeleteConfirmation')}
        </p>
      </Modal>
    </section>
  );
}
