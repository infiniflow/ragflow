import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ChevronDown, X } from 'lucide-react';
import React, { ReactNode, useState } from 'react';

export type TreeNode = {
  id: number;
  label: ReactNode;
  children?: TreeNode[];
};

type SingleSelectTreeDropdownProps = {
  allowDelete?: boolean;
  treeData: TreeNode[];
};

const SingleTreeSelect: React.FC<SingleSelectTreeDropdownProps> = ({
  allowDelete = false,
  treeData,
}) => {
  const [selectedOption, setSelectedOption] = useState<TreeNode | null>(null);

  const handleSelect = (option: TreeNode) => {
    setSelectedOption(option);
  };

  const handleDelete = (e: React.MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation();
    console.log(
      'Delete button clicked. Current selected option:',
      selectedOption,
    );
    setSelectedOption(null);
    console.log('After deletion, selected option:', selectedOption);
  };

  const renderTree = (nodes: TreeNode[]) => {
    return nodes.map((node) => (
      <div key={node.id} className="pl-4">
        <DropdownMenuItem
          onClick={() => handleSelect(node)}
          className={`flex items-center ${
            selectedOption?.id === node.id ? 'bg-gray-500' : ''
          }`}
        >
          <span>{node.label}</span>
          {node.children && (
            <ChevronDown className="ml-2 h-4 w-4 text-gray-400" />
          )}
        </DropdownMenuItem>
        {node.children && renderTree(node.children)}
      </div>
    ));
  };

  return (
    <div className="relative">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            className="flex items-center justify-between space-x-1 p-2 border rounded-md  focus:outline-none w-full"
          >
            {selectedOption ? (
              <>
                <span>{selectedOption.label}</span>
                {allowDelete && (
                  <button
                    type="button"
                    className="ml-2 text-gray-500 hover:text-red-500 focus:outline-none"
                    onClick={handleDelete}
                  >
                    <X className="h-4 w-4" />
                  </button>
                )}
              </>
            ) : (
              'Select an option'
            )}
            <ChevronDown className="h-4 w-4" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent className=" mt-2 w-56 origin-top-right rounded-md shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none">
          {renderTree(treeData)}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
};

export default SingleTreeSelect;
