import { omit } from 'lodash';

export function getQueritAgentInitialValues(
  initialValues: Record<string, any>,
) {
  return omit(initialValues, 'query', 'outputs');
}
