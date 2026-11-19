import { createEvent, fireEvent, render, screen } from '@testing-library/react';
import { CitationHoverCard } from '../citation-hover-card';

const renderCard = () =>
  render(
    <div>
      <CitationHoverCard chip={<bdi>[1]</bdi>} content={<p>chunk text</p>} />
      <button type="button">outside</button>
    </div>,
  );

const trigger = () => screen.getByText('[1]').parentElement as HTMLElement;

// jsdom has no PointerEvent, so fireEvent drops pointerType from the init dict.
const pointerUp = (element: HTMLElement, pointerType: string) => {
  const event = createEvent.pointerUp(element);
  Object.defineProperty(event, 'pointerType', { value: pointerType });
  fireEvent(element, event);
};

describe('CitationHoverCard', () => {
  it('opens on a touch tap', () => {
    renderCard();
    expect(screen.queryByText('chunk text')).toBeNull();

    pointerUp(trigger(), 'touch');

    expect(screen.getByText('chunk text')).toBeInTheDocument();
  });

  it('does not open on a mouse pointerup', () => {
    renderCard();

    pointerUp(trigger(), 'mouse');

    expect(screen.queryByText('chunk text')).toBeNull();
  });

  it('closes on Escape after a tap opened it', () => {
    renderCard();
    pointerUp(trigger(), 'touch');
    expect(screen.getByText('chunk text')).toBeInTheDocument();

    fireEvent.keyDown(document.activeElement ?? document.body, {
      key: 'Escape',
    });

    expect(screen.queryByText('chunk text')).toBeNull();
  });

  it('renders only the chip while the reference is loading', () => {
    render(<CitationHoverCard chip={<bdi>[1]</bdi>} />);

    pointerUp(trigger(), 'touch');

    expect(screen.getByText('[1]')).toBeInTheDocument();
    expect(document.querySelector('[data-radix-popper-content-wrapper]')).toBe(
      null,
    );
  });
});
