import { ButtonLoading } from '@/components/ui/button';
import { useUpdateKnowledge } from '@/hooks/use-knowledge-request';
import { useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';

export function GeneralSavingButton() {
  const form = useFormContext();
  const { saveKnowledgeConfiguration, loading: submitLoading } =
    useUpdateKnowledge();
  const { id: kb_id } = useParams();
  const { t } = useTranslation();

  const defaultValues = useMemo(
    () => form.formState.defaultValues ?? {},
    [form.formState.defaultValues],
  );
  const parser_id = defaultValues['parser_id'];

  return (
    <ButtonLoading
      type="button"
      loading={submitLoading}
      onClick={() => {
        (async () => {
          let isValidate = await form.trigger('name');
          const { name, description, permission, avatar } = form.getValues();

          if (isValidate) {
            saveKnowledgeConfiguration({
              kb_id,
              parser_id,
              name,
              description,
              avatar,
              permission,
            });
          }
        })();
      }}
    >
      {t('knowledgeConfiguration.save')}
    </ButtonLoading>
  );
}

export function SavingButton() {
  const { saveKnowledgeConfiguration, loading: submitLoading } =
    useUpdateKnowledge();
  const form = useFormContext();
  const { id: kb_id } = useParams();
  const { t } = useTranslation();

  return (
    <ButtonLoading
      loading={submitLoading}
      onClick={() => {
        (async () => {
          try {
            let beValid = await form.trigger();
            if (!beValid) {
              const errors = form.formState.errors;
              console.error('Validation errors:', errors);
            }
            if (beValid) {
              form.handleSubmit(async (values) => {
                console.log('saveKnowledgeConfiguration: ', values);
                delete values['parseType'];
                // delete values['avatar'];
                await saveKnowledgeConfiguration({
                  kb_id,
                  ...values,
                  parser_config: {
                    ...values.parser_config,
                    image_table_context_window:
                      values.parser_config.image_table_context_window,
                    image_context_size:
                      values.parser_config.image_table_context_window,
                    table_context_size:
                      values.parser_config.image_table_context_window,
                    // Unset children delimiter if this option is not enabled
                    children_delimiter: values.parser_config.enable_children
                      ? values.parser_config.children_delimiter
                      : '',
                  },
                });
              })();
            }
          } catch (e) {
            console.log(e);
          } finally {
          }
        })();
      }}
    >
      {t('knowledgeConfiguration.save')}
    </ButtonLoading>
  );
}
