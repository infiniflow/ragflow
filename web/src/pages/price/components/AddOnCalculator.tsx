import NumberInput from '@/components/originui/number-input';
import React, { useState } from 'react';

const AddOnCalculator: React.FC = () => {
  const [quantities, setQuantities] = useState<{ [key: string]: number }>({});
  const products = [
    { name: 'Storage', unit: 'GB', pricePerUnit: 1.8, per: '/mounth' },
    { name: 'Document Parsing', unit: 'page', pricePerUnit: 0.01, per: '' },
  ];

  const handleQuantityChange = (productName: string, value: number) => {
    setQuantities({ ...quantities, [productName]: value });
  };

  const totalPrice = (product: (typeof products)[0]) => {
    const total = (quantities[product.name] || 0) * product.pricePerUnit;
    return total.toFixed(2);
  };
  return (
    <div className="mt-10">
      <h2 className="text-2xl font-bold mb-4">Add-on Calculator</h2>
      <p className="mb-6">
        Estimate the cost of add-on services tailored to your needs.
      </p>
      <table
        className="w-full bg-gradient-to-r from-neutral-900 from-10%  to-black rounded-lg"
        cellPadding={15}
      >
        <thead className="text-left text-xl text-slate-300">
          <tr>
            <th className="py-2 font-normal">Product</th>
            <th className="py-2 font-normal">Plan</th>
            <th className="py-2 font-normal">Price</th>
          </tr>
        </thead>
        <tbody className="text-left p-4">
          {products.map((product) => (
            <tr key={product.name} className="border-t-gray-800 border-t-[1px]">
              <td className="w-1/3 py-4 text-white text-base">
                {product.name}
              </td>
              <td className="py-4">
                <div className="flex items-center gap-2">
                  <NumberInput
                    className="w-1/3"
                    value={quantities[product.name] || 0}
                    onChange={(e) => handleQuantityChange(product.name, e)}
                    height={40}
                  />
                  {product.unit}
                </div>
              </td>
              <td className="w-1/3 py-4">
                <span className="text-white text-2xl font-bold">
                  ${totalPrice(product)}
                </span>
                <span className="text-gray-500 text-sm ml-2">
                  {product.per}
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

export default AddOnCalculator;
