import { useUpdateArtifactPage } from '@/hooks/use-knowledge-request';
import { zodResolver } from '@hookform/resolvers/zod';
import { useCallback, useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';

import type { CommitFormValues } from '../interface';

type UseCommitArtifactParams = {
  editedContent: string;
  pageType: string;
  slug: string;
  onSuccess?: () => void;
};

export function useCommitArtifact({
  editedContent,
  pageType,
  slug,
  onSuccess,
}: UseCommitArtifactParams) {
  const { t } = useTranslation();
  const { updateArtifactPage, loading: isUpdating } = useUpdateArtifactPage();
  const [isOpen, setIsOpen] = useState(false);

  const CommitFormSchema = useMemo(
    () =>
      z.object({
        comments: z.string().min(1, {
          message: t('knowledgeDetails.versionContentRequired'),
        }),
      }),
    [t],
  );

  const form = useForm<CommitFormValues>({
    resolver: zodResolver(CommitFormSchema),
    defaultValues: { comments: '' },
  });

  const open = useCallback(() => {
    form.reset({ comments: '' });
    setIsOpen(true);
  }, [form]);

  const close = useCallback(() => {
    setIsOpen(false);
  }, []);

  const handleConfirm = useCallback(
    async (values: CommitFormValues) => {
      if (!pageType || !slug) return;

      const result = await updateArtifactPage({
        pageType,
        slug,
        body: {
          content_md: editedContent,
          comments: values.comments,
        },
      });

      if (result?.code === 0) {
        onSuccess?.();
        setIsOpen(false);
      }
    },
    [editedContent, onSuccess, pageType, slug, updateArtifactPage],
  );

  return {
    isOpen,
    open,
    close,
    form,
    handleConfirm,
    isUpdating,
  };
}
