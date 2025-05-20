import { SideDown } from '@/assets/icon/Icon';
import { Card, CardContent } from '@/components/ui/card';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
} from '@/components/ui/sidebar';
import { useMemo } from 'react';
import {
  AgentOperatorList,
  Operator,
  componentMenuList,
  operatorMap,
} from './constant';
import OperatorIcon from './operator-icon';

type OperatorItem = {
  name: Operator;
};

function OperatorCard({ name }: OperatorItem) {
  return (
    <Card className="bg-colors-background-inverse-weak  border-colors-outline-neutral-standard">
      <CardContent className="p-2 flex items-center gap-2">
        <OperatorIcon
          name={name}
          color={operatorMap[name].color}
        ></OperatorIcon>
        {name}
      </CardContent>
    </Card>
  );
}

type OperatorCollapsibleProps = { operatorList: OperatorItem[]; title: string };

function OperatorCollapsible({
  operatorList,
  title,
}: OperatorCollapsibleProps) {
  return (
    <Collapsible defaultOpen className="group/collapsible">
      <SidebarGroup>
        <SidebarGroupLabel asChild className="mb-1">
          <CollapsibleTrigger>
            <span className="font-bold text-base">{title}</span>
            <SideDown className="ml-auto" />
          </CollapsibleTrigger>
        </SidebarGroupLabel>
        <CollapsibleContent className="px-2">
          <SidebarGroupContent>
            <SidebarMenu className="gap-2">
              {operatorList.map((item) => (
                <OperatorCard key={item.name} name={item.name}></OperatorCard>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </CollapsibleContent>
      </SidebarGroup>
    </Collapsible>
  );
}

export function AgentSidebar() {
  const agentOperatorList = useMemo(() => {
    return componentMenuList.filter((x) =>
      AgentOperatorList.some((y) => y === x.name),
    );
  }, []);

  const thirdOperatorList = useMemo(() => {
    return componentMenuList.filter(
      (x) => !AgentOperatorList.some((y) => y === x.name),
    );
  }, []);

  return (
    <Sidebar variant={'floating'} className="top-16">
      <SidebarHeader>
        <p className="font-bold text-2xl">All nodes</p>
      </SidebarHeader>
      <SidebarContent>
        <OperatorCollapsible
          title="Agent operator"
          operatorList={agentOperatorList}
        ></OperatorCollapsible>
        <OperatorCollapsible
          title="Third-party tools"
          operatorList={thirdOperatorList}
        ></OperatorCollapsible>
      </SidebarContent>
    </Sidebar>
  );
}
