import { fireEvent, render, screen } from '@testing-library/react';
import React from 'react';

// `ui/button` pulls in react-router, whose dev build touches `TextEncoder` at
// import time (absent in jsdom). We only need a passthrough `Link` here.
jest.mock('react-router', () => ({
  Link: ({ children }: { children?: unknown }) => children,
}));

import EditTag from '..';

describe('EditTag Enter handling', () => {
  const setup = () => {
    const onChange = jest.fn();
    render(
      React.createElement(EditTag, {
        value: [],
        onChange,
        addButtonTestId: 'add-tag',
        inputTestId: 'tag-input',
      }),
    );
    fireEvent.click(screen.getByTestId('add-tag'));
    const input = screen.getByTestId('tag-input');
    fireEvent.change(input, { target: { value: 'foo' } });
    return { onChange, input };
  };

  it('does not confirm the tag on the Enter that ends an IME composition', () => {
    const { onChange, input } = setup();

    fireEvent.keyDown(input, { key: 'Enter', isComposing: true });

    expect(onChange).not.toHaveBeenCalled();
  });

  it('confirms the tag on a plain Enter press', () => {
    const { onChange, input } = setup();

    fireEvent.keyDown(input, { key: 'Enter' });

    expect(onChange).toHaveBeenCalledWith(['foo']);
  });
});
