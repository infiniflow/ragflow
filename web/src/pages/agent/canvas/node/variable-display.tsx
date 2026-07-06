import { ReactNode } from 'react';
import { VariableRegex } from '../../constant';

interface VariableDisplayProps {
  content: string;
  getLabel?: (value?: string) => string | ReactNode;
}

// This component mimics the VariableNode's decorate function from PromptEditor
function VariableNodeDisplay({ label }: { label: ReactNode }) {
  const content: ReactNode = (
    <span className="text-accent-primary">{label}</span>
  );

  return <div className="inline-flex items-center mr-1">{content}</div>;
}

export function VariableDisplay({ content, getLabel }: VariableDisplayProps) {
  if (!content) return null;

  // Regular expression to match content within {}
  const regex = VariableRegex;
  let match;
  let lastIndex = 0;
  const elements: ReactNode[] = [];

  const findLabelByValue = (value: string) => {
    if (getLabel) {
      const label = getLabel(value);
      return label;
    }
    return null;
  };

  while ((match = regex.exec(content)) !== null) {
    const { 1: variableValue, index, 0: template } = match;

    // Add the previous text part (if any)
    if (index > lastIndex) {
      elements.push(
        <span key={`text-${index}`}>{content.slice(lastIndex, index)}</span>,
      );
    }

    // Try to find the label
    const label = findLabelByValue(variableValue);

    if (label && label !== variableValue) {
      // If we found a valid label, render as variable node
      elements.push(
        <VariableNodeDisplay key={`variable-${index}`} label={label} />,
      );
    } else {
      // If no label found, keep as original text
      elements.push(<span key={`text-${index}-template`}>{template}</span>);
    }

    // Update index
    lastIndex = regex.lastIndex;
  }

  // Add the last part of text (if any)
  if (lastIndex < content.length) {
    elements.push(
      <span key={`text-${lastIndex}`}>{content.slice(lastIndex)}</span>,
    );
  }

  return <>{elements.length > 0 ? elements : content}</>;
}
