// pages/PricingPage.tsx
import { Building2, Gem, LucideProps, Rocket } from 'lucide-react';
import React from 'react';
import { JSX } from 'react/jsx-runtime';
import AddOnCalculator from './components/AddOnCalculator';
import FAQs from './components/FAQs';
import PricingCard from './components/PricingCard';

const PricingPage: React.FC = () => {
  const pricingPlans = [
    {
      title: 'Free',
      description:
        'Start for free and explore essential features to get your project off the ground.',
      price: '0',
      feature: {
        apps: '20',
        teamMembers: '50',
        datasetStorage: '5',
        apiRequests: '6000',
      },
      buttonLabel: 'In Use',
      isUse: true,
      icon: () => <></>,
    },
    {
      title: 'Starter',
      description:
        'Ideal for individuals and small teams starting their journey with essential features.',
      price: '9.9',
      feature: {
        apps: '40',
        teamMembers: '100',
        datasetStorage: '10',
        apiRequests: '12000',
      },
      buttonLabel: 'Upgrade Now',
      isUse: false,
      icon: (
        props?: JSX.IntrinsicAttributes &
          Omit<LucideProps, 'ref'> &
          React.RefAttributes<SVGSVGElement>,
      ) => {
        return <Rocket {...props} />;
      },
    },
    {
      title: 'Pro',
      description:
        'Perfect for growing businesses requiring more advanced tools and higher limits.',
      price: '99',
      feature: {
        apps: '80',
        teamMembers: '200',
        datasetStorage: '20',
        apiRequests: '24000',
      },
      buttonLabel: 'Upgrade Now',
      isUse: false,
      icon: (
        props?: JSX.IntrinsicAttributes &
          Omit<LucideProps, 'ref'> &
          React.RefAttributes<SVGSVGElement>,
      ) => {
        return <Gem {...props} />;
      },
    },
    {
      title: 'Enterprise',
      description:
        'Tailored for large organizations needing custom solutions, priority support, and full scalability',
      price: '?',
      feature: {
        apps: '?',
        teamMembers: '?',
        datasetStorage: '?',
        apiRequests: '?',
      },
      buttonLabel: 'Contact Us',
      isUse: false,
      icon: (
        props?: JSX.IntrinsicAttributes &
          Omit<LucideProps, 'ref'> &
          React.RefAttributes<SVGSVGElement>,
      ) => {
        return <Building2 {...props} />;
      },
    },
  ];

  const faqs = [
    {
      question: 'Can I get a refund?',
      answer:
        'We currently don’t process automatic refunds, but you can request a manual review by emailing xxxxxx@ragflow.io.',
    },
    {
      question: 'Can I get a refund?',
      answer:
        'We currently don’t process automatic refunds, but you can request a manual review by emailing xxxxxx@ragflow.io.',
    },
    {
      question: 'Can I get a refund?',
      answer:
        'We currently don’t process automatic refunds, but you can request a manual review by emailing xxxxxx@ragflow.io.',
    },
    {
      question: 'Can I get a refund?',
      answer:
        'We currently don’t process automatic refunds, but you can request a manual review by emailing xxxxxx@ragflow.io.',
    },
  ];

  return (
    <div className="min-h-screen bg-[#101015] text-white p-10 flex justify-center items-start overflow-auto h-full">
      <div className="w-[1500px]">
        <h1 className="text-[68px] leading-[80px] font-bold mb-10 text-center bg-gradient-to-r from-indigo-500 from-30% via-sky-500 via-60% to-emerald-500 bg-clip-text text-transparent">
          Scale Your Business with RAG engine
        </h1>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-10">
          {pricingPlans.map((plan, index) => (
            <PricingCard key={index} {...plan} />
          ))}
        </div>
        <AddOnCalculator />
        <FAQs faqs={faqs} />
      </div>
    </div>
  );
};

export default PricingPage;
