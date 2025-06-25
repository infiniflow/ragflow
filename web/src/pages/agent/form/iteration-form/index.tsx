import { FormContainer } from '@/components/form-container';
import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { initialRetrievalValues, VariableType } from '../../constant';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';
import { useValues } from './use-values';

const FormSchema = z.object({
  query: z.string().optional(),
  similarity_threshold: z.coerce.number(),
  keywords_similarity_weight: z.coerce.number(),
  top_n: z.coerce.number(),
  top_k: z.coerce.number(),
  kb_ids: z.array(z.string()),
  rerank_id: z.string(),
  empty_response: z.string(),
});

const IterationForm = ({ node }: INextOperatorForm) => {
  const outputList = useMemo(() => {
    return [
      {
        title: 'formalized_content',
        type: initialRetrievalValues.outputs.formalized_content.type,
      },
    ];
  }, []);

  const defaultValues = useValues(node);

  const form = useForm({
    defaultValues: defaultValues,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <form
        className="space-y-6 p-4"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <FormContainer>
          <QueryVariable
            name="items_ref"
            type={VariableType.Array}
          ></QueryVariable>
        </FormContainer>
        <Output list={outputList}></Output>
      </form>
    </Form>
  );
};

export default IterationForm;
