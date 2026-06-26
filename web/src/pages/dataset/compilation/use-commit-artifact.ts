import { useUpdateArtifactPage } from '@/hooks/use-knowledge-request';
import { IArtifactPage } from '@/interfaces/database/dataset';
import { zodResolver } from '@hookform/resolvers/zod';
import { useCallback, useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';

export type CommitFormValues = {
  comments: string;
};

type UseCommitArtifactParams = {
  editedContent: string;
  page: IArtifactPage | null | undefined;
  onSuccess?: () => void;
};

export function useCommitArtifact({
  editedContent,
  page,
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
      if (!page) return;

      const result = await updateArtifactPage({
        pageType: page.page_type,
        slug: page.slug,
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
    [editedContent, onSuccess, page, updateArtifactPage],
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
