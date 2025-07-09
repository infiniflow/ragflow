import { ChevronDown, ChevronUp } from 'lucide-react';
import React, { useState } from 'react';

interface FAQProps {
  question: string;
  answer: string;
}

const FAQ: React.FC<FAQProps> = ({ question, answer }) => {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <div
      className="bg-black p-6 rounded-lg mb-4"
      onClick={() => setIsOpen(!isOpen)}
    >
      <div className="flex justify-between items-center">
        <div className="text-xl font-bold mb-2">{question}</div>
        {!isOpen && <ChevronUp />}
        {isOpen && <ChevronDown />}
      </div>
      {isOpen && <div className="text-gray-300">{answer}</div>}
    </div>
  );
};

interface FAQsProps {
  faqs: FAQProps[];
}

const FAQs: React.FC<FAQsProps> = ({ faqs }) => {
  return (
    <div className="mt-10">
      <h2 className="text-2xl font-bold mb-4">FAQs</h2>
      <div className="flex flex-wrap gap-4">
        {faqs.map((faq, index) => (
          <div style={{ width: 'calc(50% - 1rem)' }} key={index}>
            <FAQ {...faq} />
          </div>
        ))}
      </div>
    </div>
  );
};

export default FAQs;
