import { render } from '@testing-library/react';
import { Sheet, SheetContent, SheetDescription, SheetTitle } from './sheet';

describe('SheetContent overlay behavior', () => {
  it('shows the overlay by default', () => {
    render(
      <Sheet open>
        <SheetContent>
          <SheetTitle>Test title</SheetTitle>
          <SheetDescription>Test description</SheetDescription>
        </SheetContent>
      </Sheet>,
    );

    expect(
      document.querySelector('[data-state="open"].fixed.inset-0'),
    ).toBeInTheDocument();
  });

  it('allows non-modal consumers to opt out of the overlay', () => {
    render(
      <Sheet open modal={false}>
        <SheetContent showOverlay={false}>
          <SheetTitle>Test title</SheetTitle>
          <SheetDescription>Test description</SheetDescription>
        </SheetContent>
      </Sheet>,
    );

    expect(
      document.querySelector('[data-state="open"].fixed.inset-0'),
    ).not.toBeInTheDocument();
  });
});
