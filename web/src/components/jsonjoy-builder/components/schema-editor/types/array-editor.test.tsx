import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import ArrayEditor from './array-editor';

jest.mock('../type-editor', () => ({
  __esModule: true,
  default: () => null,
}));

describe('ArrayEditor', () => {
  it('writes the latest uniqueItems switch value to the schema', async () => {
    const onChange = jest.fn();

    render(
      <ArrayEditor
        schema={{ type: 'array', items: { type: 'string' } } as any}
        validationNode={undefined}
        onChange={onChange}
      />,
    );

    await act(async () => {
      fireEvent.click(screen.getByRole('switch'));
    });

    await waitFor(() => {
      expect(onChange).toHaveBeenCalledWith(
        expect.objectContaining({ uniqueItems: true }),
      );
    });
  });
});
